package api

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/wiebe-xyz/funnelbarn/internal/bblog"
	"github.com/wiebe-xyz/funnelbarn/internal/command"
)

// Slow-evaluate thresholds (specs/012-cqrs-read-write-split/spec.md, "Async
// command dispatcher"). The dispatcher never drops a command, so a full buffer
// shows up as latency on the submitting request. An evaluate that crosses a
// threshold logs at Error level when the submit wait crossed its threshold,
// which opens a BugBarn issue, and at Warn level when only the total did.
const (
	slowEvaluateTotal      = 250 * time.Millisecond
	slowEvaluateSubmitWait = 50 * time.Millisecond
)

// slowEvaluate holds the thresholds a Server applies; tests override them.
type slowEvaluate struct {
	total      time.Duration
	submitWait time.Duration
}

var defaultSlowEvaluate = slowEvaluate{total: slowEvaluateTotal, submitWait: slowEvaluateSubmitWait}

func (s *Server) currentReadPoolWait() time.Duration {
	if s.readPoolWait == nil {
		return 0
	}
	return s.readPoolWait()
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// slowLogInterval limits the slow-evaluate Error log to one per project per
// interval. BugBarn opens an issue per event with no severity gate, and a
// stall makes every request in it slow, so one record per minute with a count
// of the suppressed ones is enough to raise the issue without flooding it.
const slowLogInterval = time.Minute

// maxTrackedSlowProjects bounds the limiter map; a reset only lets one extra
// log through per project.
const maxTrackedSlowProjects = 1024

type slowLogState struct {
	last       time.Time
	suppressed int
}

// slowLogLimiter is safe for concurrent use; its zero value is ready.
type slowLogLimiter struct {
	mu    sync.Mutex
	state map[string]*slowLogState
}

// allow reports whether a slow evaluate for projectID may log now, and how many
// were suppressed since the last one that did.
func (l *slowLogLimiter) allow(projectID string, now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state == nil || len(l.state) >= maxTrackedSlowProjects {
		l.state = make(map[string]*slowLogState)
	}
	st, ok := l.state[projectID]
	if !ok {
		l.state[projectID] = &slowLogState{last: now}
		return true, 0
	}
	if now.Sub(st.last) < slowLogInterval {
		st.suppressed++
		return false, 0
	}
	n := st.suppressed
	st.last, st.suppressed = now, 0
	return true, n
}

// reportEvaluate records the evaluate's timings on its span and logs when the
// total time or the summed submit wait crosses its threshold: at Error for the
// submit wait, at Warn for the total alone.
// poolWait is the growth of the read pool's connection wait during the request;
// the pool is shared, so it is an approximation that includes other requests.
func (s *Server) reportEvaluate(ctx context.Context, span trace.Span, projectID, flagKey string, total, submitWait, poolWait time.Duration) {
	span.SetAttributes(
		attribute.Float64("flag.duration_ms", ms(total)),
		attribute.Float64("command.submit_wait_ms", ms(submitWait)),
		attribute.Float64("db.pool.wait_ms", ms(poolWait)),
	)
	if total <= s.slow.total && submitWait <= s.slow.submitWait {
		return
	}
	// A long submit wait means the dispatcher is backed up, which is the stall
	// this log exists to raise. A slow total with a short submit wait lost
	// nothing: the caller got its value, and the time went to the host (CPU
	// contention, IO), as in FUN-48 and FUN-57. That case stays visible in the
	// logs at Warn without opening a BugBarn issue. The two levels are limited
	// separately so a Warn never uses up the slot of a later Error.
	level, limitKey := slog.LevelWarn, projectID+"/warn"
	if submitWait > s.slow.submitWait {
		level, limitKey = slog.LevelError, projectID+"/error"
	}
	ok, suppressed := s.evalLimits.slowLog.allow(limitKey, time.Now())
	if !ok {
		return
	}
	slog.Log(ctx, level, "flag evaluate slow",
		"handled", true,
		"suppressed_since_last", suppressed,
		"project_id", projectID,
		"flag_key", flagKey,
		"duration_ms", ms(total),
		"submit_wait_ms", ms(submitWait),
		"pool_wait_ms", ms(poolWait),
		"request_id", RequestIDFromContext(ctx),
	)
}

// evaluateLimits groups the per-project rate limiters of the evaluate path; its
// zero value is ready.
type evaluateLimits struct {
	slowLog    slowLogLimiter
	healthMark markThrottle
}

// flagsHealthInterval is the minimum time between queued health marks for one
// project. The health state is read in minutes, so a minute loses nothing.
const flagsHealthInterval = time.Minute

// maxTrackedHealthProjects bounds the map; a reset costs one extra mark per
// project.
const maxTrackedHealthProjects = 4096

// markThrottle remembers when each project was last marked. Its zero value is
// ready.
type markThrottle struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// due reports whether projectID is due for a mark and records it if so.
func (t *markThrottle) due(projectID string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if prev, ok := t.last[projectID]; ok && now.Sub(prev) < flagsHealthInterval {
		return false
	}
	if t.last == nil || len(t.last) >= maxTrackedHealthProjects {
		t.last = make(map[string]time.Time)
	}
	t.last[projectID] = now
	return true
}

// markFlagsEvaluated records that the project called the evaluate endpoint. It
// is queued when a dispatcher is configured; the health service's in-memory
// cache makes the applied command a cheap lookup after the first success.
//
// The mark is bookkeeping, so it never makes the request wait: at most one
// command per project per flagsHealthInterval is submitted, and the submit
// runs off the request goroutine (FUN-33: a publish to the shared Valkey took
// 60 to 740 ms on the evaluate path).
func (s *Server) markFlagsEvaluated(ctx context.Context, projectID string) {
	if s.commands != nil {
		if !s.evalLimits.healthMark.due(projectID, time.Now()) {
			return
		}
		detached := context.WithoutCancel(ctx)
		bblog.Go("flags-health-submit", func() {
			s.commands.Submit(detached, command.MarkFlagsEvaluated{ProjectID: projectID})
		})
		return
	}
	bblog.Go("flags-health", func() {
		if err := s.projectHealth.MarkFlagsEvaluated(context.Background(), projectID); err != nil {
			slog.Warn("evaluate flag: mark health", "project_id", projectID, "err", err)
		}
	})
}
