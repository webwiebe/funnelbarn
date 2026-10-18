# Spec 013: Security Headers + Rate Limiting + Request Logging

**Spec**: `specs/013-security-middleware/`
**Status**: Shipped
**Last verified against the tree**: 2026-10-17 at `ea1b0e6`
**Issues**: none found
**Principles touched**: V, VI

This spec was numbered 003 until 2026-10-17. It shared that number with `003-engineering-standards` and was renumbered to 013, the next free number.

## Purpose

The HTTP server wraps every response in security headers, limits request rates per client IP on sensitive routes, and writes one structured log line per request with a request ID.

## Functional Requirements

- **FR-001**: The system MUST set `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `X-XSS-Protection: 0`, `Referrer-Policy: strict-origin-when-cross-origin`, `Strict-Transport-Security` and a `Content-Security-Policy` on every response, including error responses. Issues: none. Tests: `internal/api/security_test.go::TestSecurityHeaders_Present`, `internal/api/security_test.go::TestSecurityHeaders_OnErrorResponse`.
- **FR-002**: The system MUST rate limit with a token bucket per client IP and answer an exhausted bucket with 429 and a `Retry-After` header. Issues: none. Tests: `internal/api/security_test.go::TestRateLimit_ResponseFormat`, `internal/api/ratelimit_test.go::TestRateLimiter_BlocksOverBurst`, `internal/api/ratelimit_test.go::TestRateLimiter_MiddlewareReturns429`.
- **FR-003**: The system MUST keep a separate bucket per client IP. Issues: none. Tests: `internal/api/ratelimit_test.go::TestRateLimiter_IndependentPerIP`.
- **FR-004**: The system MUST remove buckets that were idle for more than 10 minutes, without blocking requests. Issues: none. Tests: `internal/api/ratelimit_cleanup_test.go::TestRateLimiter_StaleBucketsAreRemoved`, `internal/api/ratelimit_cleanup_test.go::TestRateLimiter_StaleCleanup_DoesNotBlockRequests`.
- **FR-005**: The system MUST take the client IP from `X-Forwarded-For` when that header is present (from a configured trusted proxy only, once any are configured) and fall back to the remote address. Issues: none. Tests: `internal/api/ratelimit_test.go::TestClientIP_XForwardedFor`, `internal/api/ratelimit_test.go::TestClientIP_RemoteAddr_Fallback`.
- **FR-006**: The system MUST limit `POST /api/v1/login` per client IP (default 20 per minute, burst 20) and `POST /api/v1/events` per client IP (default 500 per minute, burst 100); both are configurable through `FUNNELBARN_*` variables. [inferred]
- **FR-007**: The system MUST generate an 8-byte hex request ID per request, put it in the request context, and log method, path, status, latency in milliseconds and remote address for each request. Issues: none. Tests: `internal/api/logging_test.go::TestRequestIDInContext_PropagatedToHandlers`, `internal/api/logging_test.go::TestRequestIDPropagation`.
- **FR-008**: The system MUST NOT write a submitted password to the request log. Issues: none. Tests: `internal/api/logging_test.go::TestRequestLoggerDoesNotLogPassword`.
- **FR-009**: The system MUST return a `Server-Timing` header with the handler duration. Issues: none. Tests: `internal/api/logging_test.go::TestRequestLoggerSetsServerTiming`.

## Non-Goals

- No external rate limiting dependency; the limiter is stdlib only (`internal/api/middleware.go`).

## Drift

| Claim | Source | Tree |
|---|---|---|
| Login allows 5 requests per minute, burst 5 | Acceptance criteria of the original task | `internal/config/config.go` defaults to 20 per minute, burst 20 (`FUNNELBARN_LOGIN_RATE_PER_MINUTE`, `FUNNELBARN_LOGIN_RATE_BURST`) |
| Request log fields are method, path, status, latency_ms, request_id, remote_addr | Original task section 3 | `requestLogger` also logs `query` and `user_agent` (truncated to 128 characters), logs a 5xx at error level with `handled=false`, and logs a 404 under `/api/` separately (`internal/api/middleware.go`) |
| The limiter covers login and event ingest | Original task section 2 | `internal/api/server.go` builds five limiters: login, events, API, setup and MCP; the login limiter also guards `GET /api/v1/oidc/login` |
| The task lists the files `server.go` and `middleware.go` | Original task | Both exist; the CSP is assembled by `buildCSP` and includes the IAMBarn origin when configured |

## Proof contract

- [ ] A response from any route, including a 404, carries the six security headers (`internal/api/security_test.go::TestSecurityHeaders_OnErrorResponse`).
- [ ] The first request over the burst from one IP answers 429 with `Retry-After`, while a second IP is still served (`internal/api/ratelimit_test.go::TestRateLimiter_IndependentPerIP`).
- [ ] A request log line contains the request ID and no password (`internal/api/logging_test.go::TestRequestLoggerDoesNotLogPassword`).

## Sources

- `internal/api/middleware.go`, `internal/api/server.go`, `internal/config/config.go`
- Test files named above
