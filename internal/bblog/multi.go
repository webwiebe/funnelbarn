package bblog

import (
	"context"
	"log/slog"
)

// multiHandler fans out log records to multiple slog.Handler implementations.
type multiHandler []slog.Handler

func (m multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m {
		if h.Enabled(ctx, r.Level) {
			_ = h.Handle(ctx, r.Clone())
		}
	}
	return nil
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	result := make(multiHandler, len(m))
	for i, h := range m {
		result[i] = h.WithAttrs(attrs)
	}
	return result
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	result := make(multiHandler, len(m))
	for i, h := range m {
		result[i] = h.WithGroup(name)
	}
	return result
}

// NewMultiHandler returns a slog.Handler that fans out to all provided handlers.
func NewMultiHandler(handlers ...slog.Handler) slog.Handler {
	return multiHandler(handlers)
}
