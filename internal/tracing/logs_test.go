package tracing

import (
	"context"
	"testing"
)

func TestInitLogs_DisabledWhenUnconfigured(t *testing.T) {
	// No endpoint/api key -> no exporter is created (no network), a nil handler
	// and a nil-safe shutdown are returned so the caller can skip the sink.
	handler, shutdown, err := InitLogs(context.Background(), Config{ServiceName: "test"})
	if err != nil {
		t.Fatalf("InitLogs: unexpected error: %v", err)
	}
	if handler != nil {
		t.Errorf("expected nil handler when SpanBarn unconfigured, got %v", handler)
	}
	if shutdown == nil {
		t.Fatal("expected a non-nil (no-op) shutdown func")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("no-op shutdown returned error: %v", err)
	}
}

func TestInitLogs_DisabledWhenAPIKeyMissing(t *testing.T) {
	// Endpoint present but API key empty is still treated as unconfigured.
	handler, shutdown, err := InitLogs(context.Background(), Config{
		Endpoint:    "https://spanbarn.example/v1/traces",
		ServiceName: "test",
	})
	if err != nil {
		t.Fatalf("InitLogs: unexpected error: %v", err)
	}
	if handler != nil {
		t.Errorf("expected nil handler, got %v", handler)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown")
	}
}
