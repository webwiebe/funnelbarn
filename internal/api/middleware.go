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

// securityHeaders sets response security headers, including the CSP. When
// iambarnOrigin is non-empty (scheme://host of the IAMBarn issuer), it is added
// to script-src/connect-src/img-src so the hosted IAMBarn web components can
// load their bundle and make credentialed calls back to IAMBarn. Without it the
// CSP stays strictly 'self'.
func securityHeaders(iambarnOrigin string) func(http.Handler) http.Handler {
	csp := buildCSP(iambarnOrigin)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Frame-Options", "DENY")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-XSS-Protection", "0")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			h.Set("Content-Security-Policy", csp)
			next.ServeHTTP(w, r)
		})
	}
}

// buildCSP assembles the Content-Security-Policy. The IAMBarn origin, when
// provided, is whitelisted for scripts (the widget bundle), connections (the
// components' credentialed fetch to {issuer}/api/v1/me) and images (avatars).
func buildCSP(iambarnOrigin string) string {
	script := "'self'"
	connect := "'self'"
	img := "'self' data:"
	if iambarnOrigin != "" {
		script += " " + iambarnOrigin
		connect += " " + iambarnOrigin
		img += " " + iambarnOrigin
	}
	return "default-src 'self'; script-src " + script +
		"; style-src 'self' 'unsafe-inline'; img-src " + img +
		"; connect-src " + connect
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

	if hop := firstXFF(xff); hop != "" {
		return hop
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

// limit wraps next with the given rate limiter, keyed on the trusted-proxy-aware
// client IP (s.clientIP) rather than the raw X-Forwarded-For first hop. This is
// what the public endpoints (login, ingest, setup, evaluate, OIDC) must use so
// that, when FUNNELBARN_TRUSTED_PROXIES is configured, an attacker cannot mint a
// fresh token bucket per request by spoofing X-Forwarded-For. Without trusted
// proxies configured the behaviour is unchanged (XFF is still honoured) — set
// FUNNELBARN_TRUSTED_PROXIES to your ingress/CDN to activate the protection.
func (s *Server) limit(rl *rateLimiter, next http.Handler) http.Handler {
	return rl.middlewareWithIP(next, s.clientIP)
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

// firstXFF returns the trimmed first hop from a raw X-Forwarded-For header value.
func firstXFF(xff string) string {
	if idx := strings.IndexByte(xff, ','); idx >= 0 {
		return strings.TrimSpace(xff[:idx])
	}
	return strings.TrimSpace(xff)
}

func defaultClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if hop := firstXFF(xff); hop != "" {
			return hop
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
		// Level the access log by status so 5xx (and unexpected 4xx like 413/
		// 429 on hot ingest paths) surface through the slog -> BugBarn pipe.
		// Without this, the per-handler bug from the recording-chunk 413
		// incident stays invisible at the middleware layer.
		attrs := []any{
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"status", status,
			"latency_ms", elapsed.Milliseconds(),
			"remote_addr", r.RemoteAddr,
			"user_agent", ua,
		}
		switch {
		case status >= 500:
			// handled=false: an unexpected server failure that should
			// trigger an alert. The per-handler slog.Error already gave
			// the root cause; this is the request envelope.
			attrs = append(attrs, "handled", false)
			slog.ErrorContext(ctx, "request failed", attrs...)
		case status == http.StatusRequestEntityTooLarge,
			status == http.StatusTooManyRequests:
			// 413/429 used to silently swallow real failures (the
			// recording-chunk MaxBytesReader bug). Warn surfaces them
			// to BugBarn without flooding it on every 4xx.
			attrs = append(attrs, "handled", true)
			slog.WarnContext(ctx, "request rejected", attrs...)
		default:
			slog.InfoContext(ctx, "request", attrs...)
		}

		statusStr := strconv.Itoa(status)
		metrics.HTTPRequests.WithLabelValues(r.Method, r.URL.Path, statusStr).Inc()
		metrics.HTTPLatency.WithLabelValues(r.Method, r.URL.Path).Observe(elapsed.Seconds())
	})
}
