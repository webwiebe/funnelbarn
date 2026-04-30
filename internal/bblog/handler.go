// Package bblog provides a slog.Handler wrapper that forwards Warn/Error log
// records to BugBarn as captured messages or exceptions.
//
// For Error-level records that carry an "err" attribute of type error, the
// actual error value is forwarded via bb.CaptureError so BugBarn can group
// events by stack fingerprint. All other Warn+ records use bb.CaptureMessage.
// Slog attributes from the record are forwarded as BugBarn attributes so that
// structured context (request_id, project_id, etc.) is preserved.
package bblog

import (
	"context"
	"fmt"
	"log/slog"

	bb "github.com/wiebe-xyz/bugbarn-go"
)

// Handler wraps a base slog.Handler and, for records at Warn level or above,
// also sends a capture to BugBarn with structured attributes.
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
		h.capture(ctx, record)
	}
	return h.base.Handle(ctx, record)
}

// capture forwards the record to BugBarn with all structured attributes.
func (h *Handler) capture(_ context.Context, record slog.Record) {
	attrs := make(map[string]any, record.NumAttrs())
	var capErr error

	record.Attrs(func(a slog.Attr) bool {
		// Lift any "err" attribute that is a real error for CaptureError.
		if a.Key == "err" {
			if e, ok := a.Value.Any().(error); ok {
				capErr = e
			}
		}
		attrs[a.Key] = a.Value.Any()
		return true
	})

	opts := []bb.CaptureOption{bb.WithAttributes(attrs)}

	if capErr != nil && record.Level >= slog.LevelError {
		bb.CaptureError(capErr, opts...)
	} else {
		bb.CaptureMessage(
			fmt.Sprintf("[%s] %s", record.Level, record.Message),
			opts...,
		)
	}
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{base: h.base.WithAttrs(attrs)}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{base: h.base.WithGroup(name)}
}
