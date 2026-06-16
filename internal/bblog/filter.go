package bblog

import (
	"context"
	"log/slog"
)

// FilterHandler forwards records at or above a minimum level to a wrapped
// handler, optionally dropping records matched by a predicate. It's used to send
// a filtered, lower-volume slice of logs to a secondary sink (e.g. SpanBarn)
// without affecting the primary stderr sink.
type FilterHandler struct {
	inner slog.Handler
	level slog.Level
	drop  func(slog.Record) bool
}

// NewFilterHandler wraps inner, only handling records >= level and skipping any
// record for which drop returns true (drop may be nil).
func NewFilterHandler(inner slog.Handler, level slog.Level, drop func(slog.Record) bool) *FilterHandler {
	return &FilterHandler{inner: inner, level: level, drop: drop}
}

func (h *FilterHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level && h.inner.Enabled(ctx, level)
}

func (h *FilterHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.drop != nil && h.drop(r) {
		return nil
	}
	return h.inner.Handle(ctx, r)
}

func (h *FilterHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &FilterHandler{inner: h.inner.WithAttrs(attrs), level: h.level, drop: h.drop}
}

func (h *FilterHandler) WithGroup(name string) slog.Handler {
	return &FilterHandler{inner: h.inner.WithGroup(name), level: h.level, drop: h.drop}
}
