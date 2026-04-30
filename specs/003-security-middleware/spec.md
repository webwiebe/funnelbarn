# Spec 003: Security Headers + Rate Limiting + Request Logging

## Goal
Harden the HTTP server with security headers, rate limiting on sensitive endpoints, and structured request logging.

## Files to modify
- `internal/api/server.go` — wire new middleware into ServeHTTP and registerRoutes
- `internal/api/middleware.go` (new) — security headers, rate limiting, request logging middleware

## Tasks

### 1. Security Headers Middleware
Add a `securityHeaders()` middleware that wraps every response with:
```
X-Frame-Options: DENY
X-Content-Type-Options: nosniff
X-XSS-Protection: 0
Referrer-Policy: strict-origin-when-cross-origin
Strict-Transport-Security: max-age=31536000; includeSubDomains
Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'
```

Apply to `ServeHTTP()` in server.go (after CORS, before dispatch).

### 2. Rate Limiting
Use a token bucket per IP address (stdlib only, no external deps). Implement in `middleware.go`:

```go
type rateLimiter struct {
    mu      sync.Mutex
    buckets map[string]*bucket
}
type bucket struct {
    tokens   float64
    lastSeen time.Time
}
```

- **Login endpoint** (`POST /api/v1/login`): 5 requests/minute per IP, burst 5
- **Event ingest** (`POST /api/v1/events`): 500 requests/minute per IP, burst 100
- Return `429 Too Many Requests` with `Retry-After` header on limit exceeded
- Clean up stale buckets (entries not seen in 10 minutes) on each access

Wire rate limiters into `registerRoutes()` wrapping the specific handlers.

### 3. Request Logging Middleware
Add `requestLogger()` middleware that logs each request via `slog.Info`:
- Fields: `method`, `path`, `status`, `latency_ms`, `request_id`, `remote_addr`
- Generate `request_id` as 8-byte hex (`crypto/rand`)
- Use a `responseWriter` wrapper to capture status code
- Apply globally in `ServeHTTP()` (innermost wrapper, before security headers)

## Acceptance criteria
- All HTTP responses include security headers
- `POST /api/v1/login` returns 429 after 5 rapid requests from same IP
- Every request produces a structured log line with status + latency
- No new external dependencies (stdlib only)
