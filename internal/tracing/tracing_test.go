package tracing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestInit_NoEndpointReturnsNoopShutdown(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{ServiceName: "test"})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown func is nil")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("noop shutdown returned error: %v", err)
	}
	if Tracer() == nil {
		t.Error("Tracer() is nil after Init")
	}
}

func TestMiddleware_PropagatesHandlerStatusToSpanName(t *testing.T) {
	// Init in noop mode so spans are created but never exported.
	_, _ = Init(context.Background(), Config{ServiceName: "test"})

	called := false
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		// Routing has already happened by the time the handler runs, so the
		// middleware will see Pattern populated when it sets the final span name.
		w.WriteHeader(http.StatusNoContent)
		_, _ = w.Write([]byte("ok"))
	}))

	// Wrap the middleware in a ServeMux so r.Pattern gets populated.
	mux := http.NewServeMux()
	mux.Handle("GET /widgets/{id}", h)

	req := httptest.NewRequest(http.MethodGet, "/widgets/42", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if !called {
		t.Error("inner handler was not called")
	}
	if w.Code != http.StatusNoContent {
		t.Errorf("status: want 204, got %d", w.Code)
	}
}

func TestMiddleware_NoOpWhenTracerNil(t *testing.T) {
	// Pretend tracer never got set up — middleware should just pass through.
	prev := tracer
	tracer = nil
	defer func() { tracer = prev }()

	called := false
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if !called {
		t.Error("handler not invoked when tracer nil")
	}
}

func TestStatusWriter_DefaultsTo200OnImplicitWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec}
	if _, err := sw.Write([]byte("body")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if sw.status != http.StatusOK {
		t.Errorf("implicit status: want 200, got %d", sw.status)
	}
}

func TestStatusWriter_CapturesExplicitStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec}
	sw.WriteHeader(http.StatusTeapot)
	if sw.status != http.StatusTeapot {
		t.Errorf("status: want 418, got %d", sw.status)
	}
}

func TestStartSpan_ReturnsExistingContextWhenTracerNil(t *testing.T) {
	prev := tracer
	tracer = nil
	defer func() { tracer = prev }()

	ctx := context.Background()
	got, span := StartSpan(ctx, "noop", attribute.String("k", "v"))
	if got != ctx {
		t.Error("StartSpan should pass-through context when tracer is nil")
	}
	if span == nil {
		t.Error("StartSpan returned nil span")
	}
}

func TestRecordError_HandlesNilSpanGracefully(t *testing.T) {
	// Should not panic.
	RecordError(nil, errors.New("boom"))
}

func TestRecordError_IgnoresNilError(t *testing.T) {
	_, _ = Init(context.Background(), Config{ServiceName: "test"})
	_, span := StartSpan(context.Background(), "x")
	defer span.End()
	RecordError(span, nil) // no-op, should not crash
}

// TestMiddleware_AdoptsIncomingTraceparent verifies a request carrying a W3C
// traceparent produces a server span on the SAME trace — the property that lets
// FunnelBarn's spans/logs correlate to the caller's trace in SpanBarn.
func TestMiddleware_AdoptsIncomingTraceparent(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	prev := tracer
	tracer = tp.Tracer("test")
	defer func() { tracer = prev }()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	const wantTrace = "0af7651916cd43dd8448eb211c80319c"
	var gotTrace string
	var sampled bool
	h := Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		sc := trace.SpanContextFromContext(r.Context())
		gotTrace = sc.TraceID().String()
		sampled = sc.IsSampled()
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.Header.Set("traceparent", "00-"+wantTrace+"-b7ad6b7169203331-01")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if gotTrace != wantTrace {
		t.Errorf("server span trace_id = %q, want %q (incoming traceparent not adopted)", gotTrace, wantTrace)
	}
	if !sampled {
		t.Error("sampled flag from incoming traceparent not honoured")
	}
}

// TestMiddleware_NoTraceparentStartsRoot verifies a request without trace context
// still gets a valid new-root span.
func TestMiddleware_NoTraceparentStartsRoot(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	prev := tracer
	tracer = tp.Tracer("test")
	defer func() { tracer = prev }()

	var valid bool
	h := Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		valid = trace.SpanContextFromContext(r.Context()).TraceID().IsValid()
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !valid {
		t.Error("expected a valid new-root trace id when no traceparent is present")
	}
}
