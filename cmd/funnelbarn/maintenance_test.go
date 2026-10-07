package main

import (
	"context"
	"path/filepath"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// captureSpans routes the package tracer to an in-memory exporter for one test.
func captureSpans(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	// With no endpoint Init binds the package tracer to the global provider.
	if _, err := tracing.Init(context.Background(), tracing.Config{ServiceName: "test"}); err != nil {
		t.Fatalf("tracing init: %v", err)
	}
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prev)
		_, _ = tracing.Init(context.Background(), tracing.Config{ServiceName: "test"})
	})
	return exp
}

// The pass must emit a maintenance.purge root with one child span per step, so
// a slow pass shows which DELETE held the writer.
func TestRunMaintenance_EmitsRootAndStepSpans(t *testing.T) {
	exp := captureSpans(t)
	store, err := repository.Open(filepath.Join(t.TempDir(), "maint-spans.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	cfg := config.Config{EventRetentionDays: 30, AutoRegisterTTLDays: 30}
	runMaintenance(context.Background(), cfg, store, &fakeRecordings{})

	byName := map[string]tracetest.SpanStub{}
	for _, s := range exp.GetSpans() {
		byName[s.Name] = s
	}
	root, ok := byName["maintenance.purge"]
	if !ok {
		t.Fatalf("no maintenance.purge span; got %d spans", len(byName))
	}
	if root.Parent.IsValid() {
		t.Error("maintenance.purge must be a root span")
	}
	for _, step := range []string{"web_sessions", "events", "evaluations", "auto_flags", "orphans", "recordings"} {
		child, ok := byName["maintenance.purge."+step]
		if !ok {
			t.Errorf("missing span maintenance.purge.%s", step)
			continue
		}
		if child.Parent.SpanID() != root.SpanContext.SpanID() {
			t.Errorf("maintenance.purge.%s is not a child of maintenance.purge", step)
		}
		if child.SpanContext.TraceID() != root.SpanContext.TraceID() {
			t.Errorf("maintenance.purge.%s is in a different trace", step)
		}
	}
	for _, step := range []string{"web_sessions", "events", "evaluations", "auto_flags", "orphans"} {
		if !hasAttr(byName["maintenance.purge."+step], "rows") {
			t.Errorf("maintenance.purge.%s carries no rows attribute", step)
		}
	}
}

// Steps that the config disables must not emit spans.
func TestRunMaintenance_SkippedStepsEmitNoSpans(t *testing.T) {
	exp := captureSpans(t)
	store, err := repository.Open(filepath.Join(t.TempDir(), "maint-skip.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	runMaintenance(context.Background(), config.Config{}, store, nil)

	for _, s := range exp.GetSpans() {
		switch s.Name {
		case "maintenance.purge.events", "maintenance.purge.evaluations",
			"maintenance.purge.auto_flags", "maintenance.purge.recordings":
			t.Errorf("unexpected span %s for a disabled step", s.Name)
		}
	}
}

func hasAttr(s tracetest.SpanStub, key string) bool {
	for _, a := range s.Attributes {
		if string(a.Key) == key {
			return true
		}
	}
	return false
}
