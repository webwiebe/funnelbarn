package bblog

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestMultiHandler_FansOutToAll(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	h := NewMultiHandler(
		slog.NewTextHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelInfo}),
		slog.NewTextHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelInfo}),
	)
	logger := slog.New(h)
	logger.Info("hello", "key", "value")

	for i, buf := range []*bytes.Buffer{&buf1, &buf2} {
		out := buf.String()
		if !strings.Contains(out, "hello") {
			t.Errorf("buffer %d missing message: %q", i, out)
		}
		if !strings.Contains(out, "key=value") {
			t.Errorf("buffer %d missing attr: %q", i, out)
		}
	}
}

func TestMultiHandler_Enabled(t *testing.T) {
	var buf bytes.Buffer
	h := NewMultiHandler(
		slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}),
	)
	ctx := context.Background()
	if h.Enabled(ctx, slog.LevelError) != true {
		t.Error("expected Enabled=true for Error when threshold is Warn")
	}
	if h.Enabled(ctx, slog.LevelInfo) != false {
		t.Error("expected Enabled=false for Info when threshold is Warn")
	}
}

func TestMultiHandler_RespectsPerHandlerLevel(t *testing.T) {
	var infoBuf, errBuf bytes.Buffer
	h := NewMultiHandler(
		slog.NewTextHandler(&infoBuf, &slog.HandlerOptions{Level: slog.LevelInfo}),
		slog.NewTextHandler(&errBuf, &slog.HandlerOptions{Level: slog.LevelError}),
	)
	logger := slog.New(h)
	logger.Info("info-only")

	if !strings.Contains(infoBuf.String(), "info-only") {
		t.Errorf("info handler should have received the record: %q", infoBuf.String())
	}
	if strings.Contains(errBuf.String(), "info-only") {
		t.Errorf("error handler should have filtered out the Info record: %q", errBuf.String())
	}
}

func TestMultiHandler_WithAttrsAndGroup(t *testing.T) {
	var buf bytes.Buffer
	base := NewMultiHandler(
		slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}),
	)
	h := base.WithAttrs([]slog.Attr{slog.String("service", "test")}).WithGroup("req")
	logger := slog.New(h)
	logger.Info("msg", "id", 42)

	out := buf.String()
	if !strings.Contains(out, "service=test") {
		t.Errorf("expected persisted attr in output: %q", out)
	}
	if !strings.Contains(out, "req.id=42") {
		t.Errorf("expected grouped attr in output: %q", out)
	}
}

func TestMultiHandler_Empty(t *testing.T) {
	h := NewMultiHandler()
	if h.Enabled(context.Background(), slog.LevelError) {
		t.Error("empty multi handler should never be enabled")
	}
	// Handle on an empty handler must not panic.
	logger := slog.New(h)
	logger.Error("nothing listens")
}
