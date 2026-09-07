package bblog_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/bblog"
)

// syncHandler is a slog.Handler that signals a channel on every record so
// tests can wait deterministically instead of sleeping.
type syncHandler struct {
	mu      sync.Mutex
	records []slog.Record
	notify  chan struct{}
}

func newSyncHandler() *syncHandler {
	return &syncHandler{notify: make(chan struct{}, 8)}
}

func (h *syncHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *syncHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, r)
	h.mu.Unlock()
	h.notify <- struct{}{}
	return nil
}

func (h *syncHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *syncHandler) WithGroup(string) slog.Handler      { return h }

func (h *syncHandler) waitForRecord(t *testing.T) slog.Record {
	t.Helper()
	select {
	case <-h.notify:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a log record")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.records[len(h.records)-1]
}

func TestGo_RunsFunction(t *testing.T) {
	done := make(chan struct{})
	bblog.Go("test-goroutine", func() {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fn was not run")
	}
}

func TestGo_RecoversPanicAndLogs(t *testing.T) {
	h := newSyncHandler()
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(prev)

	bblog.Go("panicky-goroutine", func() {
		panic("boom")
	})

	rec := h.waitForRecord(t)
	if rec.Level != slog.LevelError {
		t.Fatalf("level = %v, want Error", rec.Level)
	}

	var sawGoroutineName, sawErr bool
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == "goroutine" && a.Value.String() == "panicky-goroutine" {
			sawGoroutineName = true
		}
		if a.Key == "err" {
			sawErr = true
		}
		return true
	})
	if !sawGoroutineName {
		t.Error("expected goroutine name attribute")
	}
	if !sawErr {
		t.Error("expected err attribute carrying the panic")
	}
}

func TestGo_PanicDoesNotCrashProcess(t *testing.T) {
	// A test process still running after this is the assertion: an
	// unrecovered panic in any goroutine is always fatal to the whole
	// process, so simply reaching the end of the test proves recovery
	// worked.
	done := make(chan struct{})
	bblog.Go("panic-then-signal", func() {
		defer close(done)
		panic("should be recovered")
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine never completed")
	}
}
