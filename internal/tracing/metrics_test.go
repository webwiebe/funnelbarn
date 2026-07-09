package tracing

import (
	"context"
	"testing"
)

func TestInitMetrics_DisabledWhenUnconfigured(t *testing.T) {
	// Without an endpoint/api key InitMetrics is a no-op: no exporter, no
	// network, and a nil-safe shutdown func.
	shutdown, err := InitMetrics(context.Background(), Config{ServiceName: "test"})
	if err != nil {
		t.Fatalf("InitMetrics: unexpected error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected a non-nil (no-op) shutdown func")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("no-op shutdown returned error: %v", err)
	}
}

func TestInitMetrics_DisabledWhenAPIKeyMissing(t *testing.T) {
	shutdown, err := InitMetrics(context.Background(), Config{
		Endpoint:    "https://spanbarn.example/v1/traces",
		ServiceName: "test",
	})
	if err != nil {
		t.Fatalf("InitMetrics: unexpected error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("no-op shutdown returned error: %v", err)
	}
}
