package bblog

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// capture is a minimal slog.Handler that records what it was asked to handle.
type capture struct {
	enabledMin slog.Level
	handled    []string
}

func (c *capture) Enabled(_ context.Context, l slog.Level) bool { return l >= c.enabledMin }
func (c *capture) Handle(_ context.Context, r slog.Record) error {
	c.handled = append(c.handled, r.Message)
	return nil
}
func (c *capture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *capture) WithGroup(string) slog.Handler      { return c }

func TestFilterHandler_LevelGate(t *testing.T) {
	inner := &capture{}
	h := NewFilterHandler(inner, slog.LevelInfo, nil)

	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("debug must be filtered out below min level")
	}
	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("info must be enabled at min level")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("error must be enabled above min level")
	}
}

func TestFilterHandler_DropPredicate(t *testing.T) {
	inner := &capture{}
	drop := func(r slog.Record) bool { return r.Message == "request" && hasKubeProbe(r) }
	h := NewFilterHandler(inner, slog.LevelInfo, drop)

	// A health-probe request record -> dropped.
	probe := slog.NewRecord(time.Now(), slog.LevelInfo, "request", 0)
	probe.AddAttrs(slog.String("user_agent", "kube-probe/1.34"))
	_ = h.Handle(context.Background(), probe)

	// A real request record -> kept.
	real := slog.NewRecord(time.Now(), slog.LevelInfo, "request", 0)
	real.AddAttrs(slog.String("user_agent", "Mozilla/5.0"))
	_ = h.Handle(context.Background(), real)

	// A non-request record -> kept.
	other := slog.NewRecord(time.Now(), slog.LevelError, "boom", 0)
	_ = h.Handle(context.Background(), other)

	if len(inner.handled) != 2 {
		t.Fatalf("want 2 forwarded (real request + boom), got %d: %v", len(inner.handled), inner.handled)
	}
}

// hasKubeProbe mirrors the predicate used by the app; duplicated here to keep the
// test self-contained.
func hasKubeProbe(r slog.Record) bool {
	found := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "user_agent" && len(a.Value.String()) >= 10 && a.Value.String()[:10] == "kube-probe" {
			found = true
			return false
		}
		return true
	})
	return found
}
