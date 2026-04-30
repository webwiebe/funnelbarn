package bblog_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/bblog"
)

// recordingHandler is a simple slog.Handler that captures all records.
type recordingHandler struct {
	records []slog.Record
	level   slog.Level
}

func (h *recordingHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}

func (h *recordingHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(name string) slog.Handler       { return h }

// ---------------------------------------------------------------------------
// Pass-through and enabling
// ---------------------------------------------------------------------------

func TestHandler_PassesThrough(t *testing.T) {
	rec := &recordingHandler{level: slog.LevelDebug}
	h := bblog.NewHandler(rec)
	logger := slog.New(h)

	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	if len(rec.records) != 3 {
		t.Errorf("want 3 records passed through, got %d", len(rec.records))
	}
}

func TestHandler_Enabled_DelegatesToBase(t *testing.T) {
	rec := &recordingHandler{level: slog.LevelWarn}
	h := bblog.NewHandler(rec)

	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Debug should not be enabled when base level is Warn")
	}
	if !h.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("Warn should be enabled")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("Error should be enabled")
	}
}

// ---------------------------------------------------------------------------
// Capture branches in bblog.capture
// ---------------------------------------------------------------------------

func TestHandler_CaptureError_Branch(t *testing.T) {
	// Error-level record with an "err" attribute hits the CaptureError branch.
	// bb is not initialised so CaptureError is a no-op, but the branch is covered.
	rec := &recordingHandler{level: slog.LevelDebug}
	h := bblog.NewHandler(rec)
	logger := slog.New(h)

	logger.Error("database error", "err", errors.New("connection refused"))

	if len(rec.records) != 1 {
		t.Errorf("want record passed to base handler, got %d", len(rec.records))
	}
}

func TestHandler_WarnNoError_CaptureMessage_Branch(t *testing.T) {
	// Warn-level record with no "err" attribute hits the CaptureMessage branch.
	rec := &recordingHandler{level: slog.LevelDebug}
	h := bblog.NewHandler(rec)
	logger := slog.New(h)

	logger.Warn("something suspicious", "user_id", "u123", "action", "delete")

	if len(rec.records) != 1 {
		t.Errorf("want 1 record, got %d", len(rec.records))
	}
}

func TestHandler_ErrorWithNonErrorAttr_CaptureMessage_Branch(t *testing.T) {
	// Error-level but "err" attr is not an error type → falls to CaptureMessage.
	rec := &recordingHandler{level: slog.LevelDebug}
	h := bblog.NewHandler(rec)
	logger := slog.New(h)

	logger.Error("not an error object", "err", "just a string")

	if len(rec.records) != 1 {
		t.Errorf("want 1 record, got %d", len(rec.records))
	}
}

func TestHandler_InfoNotCaptured(t *testing.T) {
	// Info-level records go to base but must NOT trigger BugBarn capture.
	// We verify no panic occurs and the record still reaches the base handler.
	rec := &recordingHandler{level: slog.LevelDebug}
	h := bblog.NewHandler(rec)
	slog.New(h).Info("routine info", "key", "value")

	if len(rec.records) != 1 {
		t.Errorf("want 1 record, got %d", len(rec.records))
	}
}

// ---------------------------------------------------------------------------
// WithAttrs / WithGroup
// ---------------------------------------------------------------------------

func TestHandler_WithAttrs_ReturnsUsableHandler(t *testing.T) {
	rec := &recordingHandler{level: slog.LevelDebug}
	h := bblog.NewHandler(rec)
	h2 := h.WithAttrs([]slog.Attr{slog.String("service", "api")})
	if h2 == nil {
		t.Fatal("WithAttrs returned nil")
	}
	slog.New(h2).Info("attr test")
}

func TestHandler_WithGroup_ReturnsUsableHandler(t *testing.T) {
	rec := &recordingHandler{level: slog.LevelDebug}
	h := bblog.NewHandler(rec)
	h2 := h.WithGroup("request")
	if h2 == nil {
		t.Fatal("WithGroup returned nil")
	}
}

// ---------------------------------------------------------------------------
// Integration: JSON output passes through
// ---------------------------------------------------------------------------

func TestHandler_OutputJSON(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := bblog.NewHandler(base)
	slog.New(h).Info("structured", "key", "value")

	if buf.Len() == 0 {
		t.Error("expected JSON output in buffer")
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"structured"`)) {
		t.Errorf("expected message in JSON output: %s", buf.String())
	}
}
