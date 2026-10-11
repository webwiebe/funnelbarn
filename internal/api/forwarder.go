package api

import (
	"crypto/subtle"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// HeaderInternalToken carries FUNNELBARN_INTERNAL_TOKEN on a request a reader
// forwards to the writer (spec 012).
const HeaderInternalToken = "X-Funnelbarn-Internal-Token"

// forwardTimeout bounds one forwarded request, as in BugBarn's WriteForwarder.
const forwardTimeout = 30 * time.Second

// readerPOSTs are the non-GET routes a reader answers itself: each publishes
// its writes to Valkey (ingest, recording chunks, evaluate bookkeeping) or
// writes nothing to the database (telemetry, client errors).
var readerPOSTs = map[string]bool{
	"/api/v1/events":           true,
	"/api/v1/recordings/chunk": true,
	"/api/v1/evaluate":         true,
	"/api/v1/telemetry":        true,
	"/api/v1/client-errors":    true,
	githubWebhookPath:          true,
}

// writingGETs are GET routes that write sessions: the OIDC callback creates
// one, the logged-out landing deletes one.
var writingGETs = map[string]bool{
	"/api/v1/oidc/callback":        true,
	"/api/v1/auth/oidc/logged-out": true,
}

// setupPathPrefix is GET /api/v1/setup/{slug}, which creates a pending project
// and upserts its ingest key.
const setupPathPrefix = "/api/v1/setup/"

// forwardsToWriter reports whether a reader hands r to the writer. MCP goes
// to the writer whole: its Streamable HTTP sessions live in one process, and
// its setup guide tool writes like GET /api/v1/setup does.
func forwardsToWriter(r *http.Request) bool {
	path := r.URL.Path
	if path == mcpPath {
		return true
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return writingGETs[path] || strings.HasPrefix(path, setupPathPrefix)
	default:
		return !readerPOSTs[path]
	}
}

// corsResponseHeaders are set by the reader's ServeHTTP. Copying the writer's
// as well would send two Access-Control-Allow-Origin values, which the Fetch
// spec rejects.
var corsResponseHeaders = map[string]bool{
	"Access-Control-Allow-Origin":      true,
	"Access-Control-Allow-Headers":     true,
	"Access-Control-Allow-Methods":     true,
	"Access-Control-Allow-Credentials": true,
	"Access-Control-Expose-Headers":    true,
	"Access-Control-Max-Age":           true,
}

// writeForwarder proxies a reader's writing requests to the writer, after
// BugBarn's WriteForwarder.
type writeForwarder struct {
	writerURL string
	token     string
	client    *http.Client
}

func newWriteForwarder(writerURL, token string) *writeForwarder {
	return &writeForwarder{
		writerURL: writerURL,
		token:     token,
		client: &http.Client{
			Timeout: forwardTimeout,
			// The writer's redirects (the OIDC callback's 302) belong to the
			// browser.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// forward sends r to the writer and copies its answer back. The writer sees
// the client's address, scheme and host as this reader resolved them, which
// it believes because the request carries the internal token.
func (s *Server) forward(w http.ResponseWriter, r *http.Request) {
	f := s.forwarder
	ctx, span := tracing.StartSpan(r.Context(), "reader.forward_to_writer")
	defer span.End()
	span.SetAttributes(attribute.String("http.method", r.Method), attribute.String("http.route", r.URL.Path))

	req, err := http.NewRequestWithContext(ctx, r.Method, f.writerURL+r.URL.RequestURI(), r.Body)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		jsonError(w, "writer unavailable", http.StatusBadGateway)
		return
	}
	req.Header = r.Header.Clone()
	req.Host = r.Host
	req.ContentLength = r.ContentLength
	proto := "http"
	if s.isSecureRequest(r) {
		proto = "https"
	}
	req.Header.Set("X-Forwarded-For", s.clientIP(r))
	req.Header.Set("X-Forwarded-Proto", proto)
	req.Header.Set("X-Forwarded-Host", s.requestHost(r))
	req.Header.Set(HeaderInternalToken, f.token)

	resp, err := f.client.Do(req)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		slog.WarnContext(ctx, "forward to writer failed", "method", r.Method, "path", r.URL.Path, "err", err)
		jsonError(w, "writer unavailable", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	if resp.StatusCode >= 500 {
		span.SetStatus(codes.Error, http.StatusText(resp.StatusCode))
	}

	// The writer's headers replace the reader's own (security headers, Vary),
	// except CORS, which the reader has already decided.
	for key, vals := range resp.Header {
		if !corsResponseHeaders[key] {
			w.Header()[key] = vals
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// fromReader reports whether r was forwarded by one of this deployment's
// readers, so its X-Forwarded-* headers carry what the reader resolved.
func (s *Server) fromReader(r *http.Request) bool {
	if s.internalToken == "" {
		return false
	}
	got := r.Header.Get(HeaderInternalToken)
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(s.internalToken)) == 1
}

// remoteIP is the address of the direct peer.
func remoteIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
