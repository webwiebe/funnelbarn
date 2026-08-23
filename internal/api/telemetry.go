package api

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// The dashboard is instrumented, but a browser must never hold a telemetry API
// key: anything shipped to the page is readable by anyone who opens devtools.
// These two endpoints are the session-authenticated seam — the browser posts to
// its own origin with the cookie it already has, and the server forwards using
// credentials that never leave the cluster.

const (
	// maxTelemetryBodyBytes bounds a span batch. The instrumentation flushes at
	// 50 spans, so this is roughly 20x headroom.
	maxTelemetryBodyBytes = 256 << 10
	// maxSpansPerBatch bounds what one request may enqueue regardless of size.
	maxSpansPerBatch = 200
	// maxClientErrorField truncates browser-supplied strings. A stack from a
	// minified bundle can be long, and this is logged, not stored.
	maxClientErrorField = 4096
)

// handleTelemetry accepts a batch of spans from the dashboard's instrumentation
// and relays them to SpanBarn, so a trace shows the browser half — the page
// span and the client-side view of each API call — above the server spans that
// already join it via the traceparent header.
func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxTelemetryBodyBytes)

	var body struct {
		Spans []tracing.BrowserSpan `json:"spans"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if len(body.Spans) > maxSpansPerBatch {
		body.Spans = body.Spans[:maxSpansPerBatch]
	}

	valid := body.Spans[:0]
	for _, sp := range body.Spans {
		if validBrowserSpan(sp) {
			valid = append(valid, sp)
		}
	}

	// Accepted either way. Telemetry is not worth an error the dashboard has to
	// handle, and a 4xx here would show up in the very instrumentation that
	// produced it.
	s.spanRelay.Enqueue(valid)
	w.WriteHeader(http.StatusAccepted)
}

// validBrowserSpan rejects spans that could not join a trace anyway. IDs are
// checked for shape rather than trusted: they are browser-supplied and end up
// in SpanBarn's index.
func validBrowserSpan(sp tracing.BrowserSpan) bool {
	return isHex(sp.TraceID, 32) && isHex(sp.SpanID, 16) &&
		(sp.ParentSpanID == "" || isHex(sp.ParentSpanID, 16)) &&
		sp.Name != "" && sp.StartTime > 0
}

func isHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// handleClientError records a failure the browser saw. It logs at error level,
// which is what the BugBarn slog handler forwards, so a dashboard error becomes
// an issue without the page ever holding a BugBarn key.
func (s *Server) handleClientError(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxTelemetryBodyBytes)

	var body struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Stack   string `json:"stack"`
		URL     string `json:"url"`
		TraceID string `json:"trace_id"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		jsonError(w, "message is required", http.StatusUnprocessableEntity)
		return
	}

	// handled=false so this is triaged as a real defect: by the time the
	// browser reports it, a user has already seen it.
	slog.ErrorContext(r.Context(), "dashboard client error",
		"err", truncate(body.Message, maxClientErrorField),
		"handled", false,
		"error_type", truncate(body.Type, 128),
		"stack", truncate(body.Stack, maxClientErrorField),
		"page_url", truncate(body.URL, 2048),
		"trace_id", truncate(body.TraceID, 32),
		"user_agent", r.Header.Get("User-Agent"),
		"request_id", RequestIDFromContext(r.Context()),
	)
	w.WriteHeader(http.StatusAccepted)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
