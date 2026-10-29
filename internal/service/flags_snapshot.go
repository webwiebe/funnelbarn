package service

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/snapshot"
)

// FlagSnapshotInterval is how often a reader reloads its flags. A flag edit
// reaches evaluate within this interval plus the reader's own replication lag.
const FlagSnapshotInterval = 2 * time.Second

// flagSnapshotTimeout bounds one reload, so a stalled read is abandoned and
// retried on a later tick.
const flagSnapshotTimeout = 10 * time.Second

type flagRef struct{ projectID, flagKey string }

// FlagSnapshot keeps every flag in memory for the evaluate path. During a
// database stall it keeps serving the last good load; it has no staleness cap,
// because a stale flag value is the same answer the SDK got a moment earlier.
type FlagSnapshot struct {
	snap *snapshot.Snapshot[map[flagRef]repository.FeatureFlag]
}

// NewFlagSnapshot returns a cold snapshot that loads with list. Start it with
// Run; until the first load succeeds, evaluate reads SQLite.
func NewFlagSnapshot(list func(context.Context) ([]repository.FeatureFlag, error), now func() time.Time) *FlagSnapshot {
	return &FlagSnapshot{snap: snapshot.New(snapshot.Config[map[flagRef]repository.FeatureFlag]{
		Name:     "flags",
		Interval: FlagSnapshotInterval,
		Timeout:  flagSnapshotTimeout,
		Now:      now,
		Load: func(ctx context.Context) (map[flagRef]repository.FeatureFlag, error) {
			flags, err := list(ctx)
			if err != nil {
				return nil, err
			}
			m := make(map[flagRef]repository.FeatureFlag, len(flags))
			for _, f := range flags {
				m[flagRef{f.ProjectID, f.FlagKey}] = f
			}
			return m, nil
		},
	})}
}

// Run reloads the snapshot in the background until ctx is done.
func (fs *FlagSnapshot) Run(ctx context.Context) { fs.snap.Run(ctx) }

// Refresh reloads the snapshot once.
func (fs *FlagSnapshot) Refresh(ctx context.Context) error { return fs.snap.Refresh(ctx) }

// Age is the time since the last good load.
func (fs *FlagSnapshot) Age() time.Duration { return fs.snap.Age() }

// flag returns the held flag. ok is false when the snapshot is cold or does not
// hold the flag, and the caller then reads SQLite: a flag created since the
// last load, or auto-registered by this request, is only there.
func (fs *FlagSnapshot) flag(projectID, flagKey string) (repository.FeatureFlag, bool) {
	m, _, ok := fs.snap.Get()
	if !ok {
		return repository.FeatureFlag{}, false
	}
	f, ok := m[flagRef{projectID, flagKey}]
	return f, ok
}

// WithSnapshot makes evaluation read flags from fs first.
func (svc *FlagService) WithSnapshot(fs *FlagSnapshot) *FlagService {
	svc.snapshot = fs
	return svc
}

// flagForEvaluate reads the flag from the snapshot, or from SQLite when the
// snapshot does not hold it, and records which on the span as flag.source.
func (svc *FlagService) flagForEvaluate(ctx context.Context, projectID, flagKey string) (repository.FeatureFlag, error) {
	span := trace.SpanFromContext(ctx)
	if svc.snapshot != nil {
		if f, ok := svc.snapshot.flag(projectID, flagKey); ok {
			span.SetAttributes(attribute.String("flag.source", "snapshot"))
			return f, nil
		}
	}
	span.SetAttributes(attribute.String("flag.source", "db"))
	return svc.store.FlagByKey(ctx, projectID, flagKey)
}
