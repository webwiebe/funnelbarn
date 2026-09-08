// Package bblog provides a slog.Handler wrapper that forwards Error-level log
// records to BugBarn as captured errors or messages.
//
// For records that carry an "err" attribute of type error, the actual error
// value is forwarded via bb.CaptureError so BugBarn can group events by stack
// fingerprint. Every other Error record uses bb.CaptureMessage. Slog attributes
// from the record are forwarded as BugBarn attributes so that structured
// context (request_id, project_id, etc.) is preserved.
//
// Warn records are deliberately not forwarded, despite what several call sites
// used to assume. BugBarn opens an issue for every event it ingests — there is
// no severity gate on the way in — so a warning sent there is indistinguishable
// from a crash: it opens an issue, accrues events, and has to be resolved by
// hand. The things this service warns about are overwhelmingly things its
// callers did (a crawler with no API key, a key whose project was deleted, a
// doubled request path), and six of the eleven open FunnelBarn issues on
// 2026-09-08 were warnings of exactly that kind, three of them still arriving
// from AhrefsBot.
//
// Warnings still go to stderr and to SpanBarn. The rates worth alerting on have
// Prometheus counters instead — funnelbarn_events_rejected_total and
// funnelbarn_misrouted_requests_total — which is where a surge belongs: a
// counter shows the rate, whereas an issue only ever says "happened again".
package bblog

import (
	"context"
	"fmt"
	"log/slog"

	bb "github.com/wiebe-xyz/bugbarn-go"
)

// Handler wraps a base slog.Handler and, for records at Error level or above,
// also sends a capture to BugBarn with structured attributes.
type Handler struct {
	base slog.Handler
}

// NewHandler returns a Handler that passes all records to base and additionally
// captures Error+ records via BugBarn.
func NewHandler(base slog.Handler) *Handler {
	return &Handler{base: base}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	if record.Level >= slog.LevelError {
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

	// Only Error+ records reach here, so no level test is needed: an "err"
	// attribute that is a real error always gets the stack-fingerprinted path.
	if capErr != nil {
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
