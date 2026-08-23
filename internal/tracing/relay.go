package tracing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// BrowserSpan is one span produced by the dashboard's own instrumentation.
// The shape is SpanBarn's native JSON ingest format, so a batch is relayed
// through untouched rather than round-tripped via the OTel SDK — which cannot
// emit a span with a caller-chosen span ID, and a browser CLIENT span must keep
// the exact ID it put in the traceparent header or the server span it parents
// would not join up.
type BrowserSpan struct {
	TraceID      string         `json:"traceId"`
	SpanID       string         `json:"spanId"`
	ParentSpanID string         `json:"parentSpanId,omitempty"`
	Name         string         `json:"name"`
	Service      string         `json:"service"`
	Kind         string         `json:"kind"`
	Status       string         `json:"status"`
	StartTime    int64          `json:"startTime"` // microseconds since epoch
	Duration     int64          `json:"duration"`  // microseconds
	Attributes   map[string]any `json:"attributes,omitempty"`
}

// spansURL derives SpanBarn's native span-ingest URL from the configured OTLP
// traces endpoint (".../v1/traces" -> ".../api/v1/spans"). The native endpoint
// takes the same JSON the browser produces, so no OTLP encoding is involved.
func spansURL(tracesEndpoint string) string {
	base := strings.TrimSuffix(strings.TrimRight(tracesEndpoint, "/"), "/v1/traces")
	return strings.TrimRight(base, "/") + "/api/v1/spans"
}

// SpanRelay forwards browser spans to SpanBarn.
//
// It is deliberately fire-and-forget with a bounded queue: telemetry must never
// slow down or fail a dashboard request, and a SpanBarn outage must cost a few
// dropped spans rather than back-pressure into the API.
type SpanRelay struct {
	url    string
	apiKey string
	client *http.Client

	queue chan []BrowserSpan
	wg    sync.WaitGroup

	mu      sync.Mutex
	dropped int
}

// NewSpanRelay returns a relay, or nil when telemetry export is not configured
// (a nil *SpanRelay is safe to call — Enqueue is a no-op).
func NewSpanRelay(cfg Config) *SpanRelay {
	if cfg.Endpoint == "" || cfg.APIKey == "" {
		return nil
	}
	r := &SpanRelay{
		url:    spansURL(cfg.Endpoint),
		apiKey: cfg.APIKey,
		client: &http.Client{Timeout: 5 * time.Second},
		queue:  make(chan []BrowserSpan, 64),
	}
	r.wg.Add(1)
	go r.run()
	return r
}

// Enabled reports whether spans will actually be forwarded.
func (r *SpanRelay) Enabled() bool { return r != nil }

// Enqueue hands a batch to the relay. It never blocks: a full queue drops the
// batch and counts it, because holding the request open to deliver telemetry
// would make the dashboard slower exactly when the backend is already strained.
func (r *SpanRelay) Enqueue(spans []BrowserSpan) {
	if r == nil || len(spans) == 0 {
		return
	}
	select {
	case r.queue <- spans:
	default:
		r.mu.Lock()
		r.dropped += len(spans)
		n := r.dropped
		r.mu.Unlock()
		slog.Warn("telemetry relay queue full, dropping browser spans",
			"handled", true, "spans", len(spans), "dropped_total", n)
	}
}

// Dropped returns how many spans have been dropped for lack of queue space.
func (r *SpanRelay) Dropped() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dropped
}

func (r *SpanRelay) run() {
	defer r.wg.Done()
	for batch := range r.queue {
		if err := r.send(batch); err != nil {
			// Warn, not error: losing browser telemetry is not a user-visible
			// fault, and erroring here would report a SpanBarn outage into
			// BugBarn once per batch.
			slog.Warn("telemetry relay send failed", "err", err, "handled", true, "spans", len(batch))
		}
	}
}

func (r *SpanRelay) send(spans []BrowserSpan) error {
	body, err := json.Marshal(map[string]any{"spans": spans})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("spanbarn ingest returned %s", resp.Status)
	}
	return nil
}

// Shutdown stops the relay after draining what is already queued.
func (r *SpanRelay) Shutdown() {
	if r == nil {
		return
	}
	close(r.queue)
	r.wg.Wait()
}
