package api

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"
)

// recordingHandler keeps every slog record so a test can look for the slow-evaluate ones.
type recordingHandler struct {
	mu   sync.Mutex
	recs []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recs = append(h.recs, r)
	return nil
}
func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordingHandler) slowAt(level slog.Level) []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []slog.Record
	for _, r := range h.recs {
		if r.Level == level && r.Message == "flag evaluate slow" {
			out = append(out, r)
		}
	}
	return out
}

func captureSlog(t *testing.T) *recordingHandler {
	t.Helper()
	h := &recordingHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return h
}

func recordAttrs(r slog.Record) map[string]any {
	m := map[string]any{}
	r.Attrs(func(a slog.Attr) bool { m[a.Key] = a.Value.Any(); return true })
	return m
}

// A slow total with a short submit wait lost nothing, so it logs at Warn and
// opens no BugBarn issue (FUN-48, FUN-57).
func TestEvaluate_SlowTotalLogsAtWarn(t *testing.T) {
	h := captureSlog(t)
	srv, store := newAuthedServer(t)
	srv.slow = slowEvaluate{total: time.Nanosecond, submitWait: time.Hour}
	p, flag, cookie, csrf := playgroundFlag(t, srv, store, "slow")

	w := postJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags/evaluate", map[string]any{
		"flag_key": flag.FlagKey,
		"context":  map[string]any{"user_id": "u1"},
	}, cookie, csrf)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}

	if n := len(h.slowAt(slog.LevelError)); n != 0 {
		t.Fatalf("a slow total alone must not log at Error, got %d", n)
	}
	recs := h.slowAt(slog.LevelWarn)
	if len(recs) != 1 {
		t.Fatalf("want 1 slow-evaluate Warn log, got %d", len(recs))
	}
	attrs := recordAttrs(recs[0])
	for _, key := range []string{"handled", "project_id", "flag_key", "duration_ms", "submit_wait_ms"} {
		if _, ok := attrs[key]; !ok {
			t.Errorf("slow-evaluate log is missing %q", key)
		}
	}
	if attrs["handled"] != true || attrs["project_id"] != p.ID || attrs["flag_key"] != flag.FlagKey {
		t.Errorf("unexpected attrs: %v", attrs)
	}
}

func TestEvaluate_FastDoesNotLogSlow(t *testing.T) {
	h := captureSlog(t)
	srv, store := newAuthedServer(t)
	srv.slow = slowEvaluate{total: time.Hour, submitWait: time.Hour}
	p, flag, cookie, csrf := playgroundFlag(t, srv, store, "fast")

	postJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags/evaluate", map[string]any{
		"flag_key": flag.FlagKey,
		"context":  map[string]any{"user_id": "u1"},
	}, cookie, csrf)

	if n := len(h.slowAt(slog.LevelError)) + len(h.slowAt(slog.LevelWarn)); n != 0 {
		t.Fatalf("want no slow-evaluate log, got %d", n)
	}
}

// The submit wait alone trips the Error; a slow total alone logs at Warn.
func TestReportEvaluate_SubmitWaitThreshold(t *testing.T) {
	h := captureSlog(t)
	srv := &Server{slow: defaultSlowEvaluate}
	_, span := noop.NewTracerProvider().Tracer("t").Start(context.Background(), "s")
	errs := func() int { return len(h.slowAt(slog.LevelError)) }
	warns := func() int { return len(h.slowAt(slog.LevelWarn)) }

	srv.reportEvaluate(context.Background(), span, "p", "k", time.Millisecond, slowEvaluateSubmitWait, 0)
	if errs()+warns() != 0 {
		t.Fatalf("wait at the threshold must not log, got %d error %d warn", errs(), warns())
	}
	srv.reportEvaluate(context.Background(), span, "p", "k", time.Millisecond, slowEvaluateSubmitWait+time.Millisecond, 0)
	if errs() != 1 || warns() != 0 {
		t.Fatalf("wait over the threshold must log one Error, got %d error %d warn", errs(), warns())
	}
	srv.reportEvaluate(context.Background(), span, "p2", "k", slowEvaluateTotal+time.Millisecond, 0, 0)
	if errs() != 1 || warns() != 1 {
		t.Fatalf("total over the threshold must log one Warn, got %d error %d warn", errs(), warns())
	}
	// Same project again within slowLogInterval: suppressed per level.
	srv.reportEvaluate(context.Background(), span, "p", "k", slowEvaluateTotal+time.Millisecond, 0, 0)
	srv.reportEvaluate(context.Background(), span, "p2", "k", slowEvaluateTotal+time.Millisecond, 0, 0)
	if warns() != 2 {
		t.Fatalf("a Warn for project p is not limited by its earlier Error, and p2's repeat is, got %d warn", warns())
	}
	srv.reportEvaluate(context.Background(), span, "p", "k", time.Millisecond, slowEvaluateSubmitWait+time.Millisecond, 0)
	if errs() != 1 {
		t.Fatalf("a second Error within the interval must not log, got %d", errs())
	}
}

func TestSlowLogLimiter(t *testing.T) {
	var l slowLogLimiter
	t0 := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	if ok, n := l.allow("p", t0); !ok || n != 0 {
		t.Fatalf("first: got %v %d", ok, n)
	}
	for i := 1; i <= 3; i++ {
		if ok, _ := l.allow("p", t0.Add(time.Duration(i)*time.Second)); ok {
			t.Fatal("within the interval must be suppressed")
		}
	}
	if ok, _ := l.allow("q", t0.Add(time.Second)); !ok {
		t.Fatal("another project is limited separately")
	}
	if ok, n := l.allow("p", t0.Add(slowLogInterval)); !ok || n != 3 {
		t.Fatalf("after the interval: got %v %d, want true 3", ok, n)
	}
}
