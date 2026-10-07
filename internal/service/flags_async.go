package service

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/bblog"
	"github.com/wiebe-xyz/funnelbarn/internal/command"
)

// WithCommands routes the evaluate path's bookkeeping writes (evaluation rows,
// last_evaluated_at, auto-registration) through the dispatcher so a request
// never waits on the write connection (spec 012, phase 1). A nil dispatcher
// keeps the synchronous behaviour.
func (svc *FlagService) WithCommands(d command.Bus) *FlagService {
	svc.commands = d
	return svc
}

// SubmitWait accumulates how long one request waited to submit commands. The
// dispatcher blocks a submitter while its buffer is full and never drops, so
// this sum is the latency that backpressure added to the request.
type SubmitWait struct{ ns atomic.Int64 }

// Add records one submit wait.
func (w *SubmitWait) Add(d time.Duration) { w.ns.Add(int64(d)) }

// Total returns the summed wait.
func (w *SubmitWait) Total() time.Duration { return time.Duration(w.ns.Load()) }

type submitWaitKey struct{}

// WithSubmitWait returns a context carrying a SubmitWait, reusing the one
// already present so several steps of one request share a single sum.
func WithSubmitWait(ctx context.Context) (context.Context, *SubmitWait) {
	if w, ok := ctx.Value(submitWaitKey{}).(*SubmitWait); ok {
		return ctx, w
	}
	w := &SubmitWait{}
	return context.WithValue(ctx, submitWaitKey{}, w), w
}

// AddSubmitWait adds d to the request's SubmitWait, if its context has one.
func AddSubmitWait(ctx context.Context, d time.Duration) {
	if w, ok := ctx.Value(submitWaitKey{}).(*SubmitWait); ok {
		w.Add(d)
	}
}

// submit queues c and records the wait on the request's SubmitWait.
func (svc *FlagService) submit(ctx context.Context, c command.Command) {
	AddSubmitWait(ctx, svc.commands.Submit(ctx, c))
}

// touchInterval throttles the last_evaluated_at write to at most one per flag
// per minute. The gate it replaces was an origin filter, which limited write
// amplification by excluding the flags that generate the most evaluations —
// exactly the ones the column needs to be right about. A time throttle applies
// the same concern uniformly: staleness is measured in days, so a minute's
// resolution costs nothing and a flag served a thousand times a second still
// produces one write per minute.
const touchInterval = time.Minute

// maxTrackedTouches bounds the throttle map. Flags per instance are few (tens),
// but auto-registration can mint them, so the map is dropped rather than grown
// without limit; the only cost of losing it is one extra write per flag.
const maxTrackedTouches = 4096

// shouldTouch reports whether this flag's last_evaluated_at is due for a write,
// recording the decision so the next evaluation within touchInterval skips it.
func (svc *FlagService) shouldTouch(projectID, flagKey string, now time.Time) bool {
	key := projectID + "\x00" + flagKey
	svc.touchedMu.Lock()
	defer svc.touchedMu.Unlock()
	if last, ok := svc.touchedAt[key]; ok && now.Sub(last) < touchInterval {
		return false
	}
	if len(svc.touchedAt) >= maxTrackedTouches {
		svc.touchedAt = make(map[string]time.Time, maxTrackedTouches)
	}
	svc.touchedAt[key] = now
	return true
}

// touchEvaluated best-effort bumps last_evaluated_at for any flag, whatever its
// origin or evaluation reason, off the request path so it never adds latency or
// fails the evaluation. With a dispatcher it is a queued command.
func (svc *FlagService) touchEvaluated(ctx context.Context, projectID, flagKey string) {
	if !svc.shouldTouch(projectID, flagKey, time.Now()) {
		return
	}
	if svc.commands != nil {
		svc.submit(ctx, command.TouchFlagEvaluated{ProjectID: projectID, FlagKey: flagKey})
		return
	}
	bblog.Go("flags-touch-evaluated", func() {
		ctx := context.Background()
		f, err := svc.store.FlagByKey(ctx, projectID, flagKey)
		if err != nil {
			return
		}
		if err := svc.store.TouchFlagEvaluated(ctx, f.ID); err != nil {
			slog.WarnContext(ctx, "flag: touch last_evaluated_at", "err", err, "handled", true,
				"flag_id", f.ID, "project_id", projectID)
		}
	})
}
