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
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// Slow-evaluate thresholds (specs/012-cqrs-read-write-split/spec.md, "Async
// command dispatcher"). The dispatcher never drops a command, so a full buffer
// shows up as latency on the submitting request. An evaluate that crosses
// either threshold logs at Error level, which opens a BugBarn issue.
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

// reportEvaluate records the evaluate's timings on its span and logs at Error
// when the total time or the summed submit wait crosses its threshold.
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
	ok, suppressed := s.slowLog.allow(projectID, time.Now())
	if !ok {
		return
	}
	slog.ErrorContext(ctx, "flag evaluate slow",
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

// markFlagsEvaluated records that the project called the evaluate endpoint. It
// is queued when a dispatcher is configured; the health service's in-memory
// cache makes the applied command a cheap lookup after the first success.
func (s *Server) markFlagsEvaluated(ctx context.Context, projectID string) {
	if s.commands != nil {
		waited := s.commands.Submit(ctx, command.MarkFlagsEvaluated{ProjectID: projectID})
		service.AddSubmitWait(ctx, waited)
		return
	}
	bblog.Go("flags-health", func() {
		if err := s.projectHealth.MarkFlagsEvaluated(context.Background(), projectID); err != nil {
			slog.Warn("evaluate flag: mark health", "project_id", projectID, "err", err)
		}
	})
}
