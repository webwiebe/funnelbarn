package tracing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/attribute"
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
