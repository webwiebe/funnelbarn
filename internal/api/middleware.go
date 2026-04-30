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

// allow returns true if the request is within the rate limit, false if it
// should be rejected. It also cleans up stale buckets (idle > 10 min).
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Clean up stale buckets on every access.
	for k, b := range rl.buckets {
		if now.Sub(b.lastSeen) > 10*time.Minute {
			delete(rl.buckets, k)
		}
	}

	b, ok := rl.buckets[ip]
	if !ok {
		b = &bucket{tokens: rl.capacity, lastSeen: now}
		rl.buckets[ip] = b
	}

	// Refill tokens based on elapsed time.
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

// clientIP extracts the real client IP address from the request.
// It respects X-Forwarded-For when present (for reverse-proxy deployments),
// taking the first IP in the chain. Falls back to RemoteAddr.
func clientIP(r *http.Request) string {
	// Respect X-Forwarded-For from trusted proxies (single hop).
	// In production, a reverse proxy (Traefik/nginx) sets this.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain.
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

// middleware returns an http.Handler that enforces the rate limit.
func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)

		if !rl.allow(ip) {
			retryAfter := int(60.0 / rl.rate)
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
			jsonError(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
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
