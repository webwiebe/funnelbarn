package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
)

// --------------------------------------------------------------------------
// Request ID context propagation
// --------------------------------------------------------------------------

type contextKey string

const requestIDKey contextKey = "request_id"

// RequestIDFromContext retrieves the request ID from the context.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// --------------------------------------------------------------------------
// Security Headers Middleware
// --------------------------------------------------------------------------

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-XSS-Protection", "0")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

// --------------------------------------------------------------------------
// Rate Limiting (token bucket, per IP, stdlib only)
// --------------------------------------------------------------------------

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

type rateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     float64 // tokens per second
	capacity float64 // burst capacity
}

func newRateLimiter(perMinute, burst float64) *rateLimiter {
	return &rateLimiter{
		buckets:  make(map[string]*bucket),
		rate:     perMinute / 60.0,
		capacity: burst,
	}
}

// allow returns true if the request is within the rate limit.
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	b, ok := rl.buckets[ip]
	if !ok {
		b = &bucket{tokens: rl.capacity, lastSeen: now}
		rl.buckets[ip] = b
	}

	elapsed := now.Sub(b.lastSeen).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > rl.capacity {
		b.tokens = rl.capacity
	}
	b.lastSeen = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// cleanup removes stale buckets (idle > 10 min).
func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for k, b := range rl.buckets {
		if now.Sub(b.lastSeen) > 10*time.Minute {
			delete(rl.buckets, k)
		}
	}
}

// startCleanup runs periodic cleanup in a background goroutine until ctx is cancelled.
func (rl *rateLimiter) startCleanup(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				rl.cleanup()
			}
		}
	}()
}

// clientIP extracts the real client IP address from the request.
// When trustedProxies is configured, X-Forwarded-For is only respected if the
// direct connection comes from one of those IPs. When no trusted proxies are
// configured, X-Forwarded-For is trusted unconditionally (backwards compatible).
func (s *Server) clientIP(r *http.Request) string {
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteIP = r.RemoteAddr
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return remoteIP
	}

	if len(s.trustedProxies) > 0 && !s.isTrustedProxy(remoteIP) {
		return remoteIP
	}

	if idx := strings.IndexByte(xff, ','); idx >= 0 {
		xff = strings.TrimSpace(xff[:idx])
	} else {
		xff = strings.TrimSpace(xff)
	}
	if xff != "" {
		return xff
	}
	return remoteIP
}

func (s *Server) isTrustedProxy(ip string) bool {
	for _, trusted := range s.trustedProxies {
		if trusted == ip {
			return true
		}
	}
	return false
}

// middleware returns an http.Handler that enforces the rate limit.
// It falls back to RemoteAddr for IP extraction (used for routes not
// bound to a Server, e.g. in tests).
func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return rl.middlewareWithIP(next, defaultClientIP)
}

func (rl *rateLimiter) middlewareWithIP(next http.Handler, extractIP func(*http.Request) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)

		if !rl.allow(ip) {
			retryAfter := int(60.0 / rl.rate)
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
			jsonError(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func defaultClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx >= 0 {
			xff = strings.TrimSpace(xff[:idx])
		} else {
			xff = strings.TrimSpace(xff)
		}
		if xff != "" {
			return xff
		}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// --------------------------------------------------------------------------
// Request Logging Middleware
// --------------------------------------------------------------------------

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	return rw.ResponseWriter.Write(b)
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate 8-byte hex request ID.
		var buf [8]byte
		_, _ = rand.Read(buf[:])
		requestID := hex.EncodeToString(buf[:])

		// Propagate request ID through context so downstream handlers can use it.
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		r = r.WithContext(ctx)

		// Truncate user agent to 128 chars.
		ua := r.UserAgent()
		if len(ua) > 128 {
			ua = ua[:128]
		}

		rw := &responseWriter{ResponseWriter: w, status: 0}
		next.ServeHTTP(rw, r)

		status := rw.status
		if status == 0 {
			status = http.StatusOK
		}

		elapsed := time.Since(start)
		slog.InfoContext(ctx, "request",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"status", status,
			"latency_ms", elapsed.Milliseconds(),
			"remote_addr", r.RemoteAddr,
			"user_agent", ua,
		)

		statusStr := strconv.Itoa(status)
		metrics.HTTPRequests.WithLabelValues(r.Method, r.URL.Path, statusStr).Inc()
		metrics.HTTPLatency.WithLabelValues(r.Method, r.URL.Path).Observe(elapsed.Seconds())
	})
}
