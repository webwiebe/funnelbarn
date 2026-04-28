// Package bblog provides a slog.Handler wrapper that forwards warn/error
// log records to BugBarn as captured messages.
package bblog

import (
	"context"
	"fmt"
	"log/slog"

	bb "github.com/wiebe-xyz/bugbarn-go"
)

// Handler wraps a base slog.Handler and, for records at Warn level or above,
// also sends a CaptureMessage to BugBarn.
type Handler struct {
	base slog.Handler
}

// NewHandler returns a Handler that passes all records to base and additionally
// captures Warn+ records via BugBarn.
func NewHandler(base slog.Handler) *Handler {
	return &Handler{base: base}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	if record.Level >= slog.LevelWarn {
		bb.CaptureMessage(fmt.Sprintf("[%s] %s", record.Level, record.Message))
	}
	return h.base.Handle(ctx, record)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{base: h.base.WithAttrs(attrs)}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{base: h.base.WithGroup(name)}
}
