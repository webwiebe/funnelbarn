// Package snapshot keeps the last good result of a database read in memory and
// reloads it on a timer, so a request can answer from memory while the
// database is stalled.
//
// The layer7 disk stalls of 2026-10-29 held SQLite reads for up to 29 s. The
// flag evaluate path made two such reads per request (the API key and the
// flag), so every SDK call in the stall waited the whole time.
package snapshot

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/bblog"
)

// failureLogInterval limits the refresh-failure Warn to one per snapshot per
// interval while the failures last.
const failureLogInterval = time.Minute

// Config describes one snapshot.
type Config[T any] struct {
	// Name labels the snapshot in logs and metrics ("flags", "api_keys").
	Name string
	// Interval is the time between refreshes.
	Interval time.Duration
	// Timeout bounds one refresh. Zero means no bound beyond the caller's context.
	Timeout time.Duration
	// Load reads the full value. A load that returns an error leaves the last
	// good value in place.
	Load func(ctx context.Context) (T, error)
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

type entry[T any] struct {
	val      T
	loadedAt time.Time
}

// Snapshot holds the last value Load returned without an error. It is safe
// for concurrent use.
type Snapshot[T any] struct {
	cfg     Config[T]
	created time.Time
	cur     atomic.Pointer[entry[T]]

	refreshMu sync.Mutex // one load at a time

	logMu      sync.Mutex
	failing    bool
	lastLogged time.Time
	failures   int
}

// New returns a cold snapshot. Call Refresh or Run to load it.
func New[T any](cfg Config[T]) *Snapshot[T] {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Snapshot[T]{cfg: cfg, created: cfg.Now()}
}

// Name returns the snapshot's name.
func (s *Snapshot[T]) Name() string { return s.cfg.Name }

// Get returns the last good value and its age. ok is false while nothing has
// loaded yet.
func (s *Snapshot[T]) Get() (val T, age time.Duration, ok bool) {
	e := s.cur.Load()
	if e == nil {
		return val, 0, false
	}
	return e.val, s.cfg.Now().Sub(e.loadedAt), true
}

// Age is the time since the last good load, or since New when nothing has
// loaded yet. It is what the snapshot-age metric reports.
func (s *Snapshot[T]) Age() time.Duration {
	if e := s.cur.Load(); e != nil {
		return s.cfg.Now().Sub(e.loadedAt)
	}
	return s.cfg.Now().Sub(s.created)
}

// Refresh loads the value once. On success it replaces the held value; on
// failure the held value stays and the error is returned.
func (s *Snapshot[T]) Refresh(ctx context.Context) error {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if s.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.cfg.Timeout)
		defer cancel()
	}
	// The load time is taken before the read: a value read during a long
	// stall is only as fresh as the moment the read started.
	started := s.cfg.Now()
	val, err := s.cfg.Load(ctx)
	if err != nil {
		s.noteFailure(ctx, err)
		return err
	}
	s.cur.Store(&entry[T]{val: val, loadedAt: started})
	s.noteSuccess(ctx)
	return nil
}

// Run refreshes now and then every Interval until ctx is done. It runs on its
// own goroutine, so a stalled load never holds a request.
func (s *Snapshot[T]) Run(ctx context.Context) {
	bblog.Go("snapshot-"+s.cfg.Name, func() {
		ticker := time.NewTicker(s.cfg.Interval)
		defer ticker.Stop()
		for {
			_ = s.Refresh(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
}

// noteFailure logs at Warn: the snapshot keeps serving its last good value,
// so a failed refresh loses nothing yet. The age shows how stale that value is.
func (s *Snapshot[T]) noteFailure(ctx context.Context, err error) {
	if ctx.Err() == context.Canceled {
		return // shutdown
	}
	s.logMu.Lock()
	defer s.logMu.Unlock()
	now := s.cfg.Now()
	s.failing = true
	s.failures++
	if !s.lastLogged.IsZero() && now.Sub(s.lastLogged) < failureLogInterval {
		return
	}
	s.lastLogged = now
	slog.WarnContext(ctx, "snapshot refresh failed",
		"handled", true,
		"snapshot", s.cfg.Name,
		"err", err,
		"failures", s.failures,
		"age_ms", s.Age().Milliseconds(),
	)
}

func (s *Snapshot[T]) noteSuccess(ctx context.Context) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	if !s.failing {
		return
	}
	slog.InfoContext(ctx, "snapshot refresh recovered",
		"snapshot", s.cfg.Name,
		"failures", s.failures,
	)
	s.failing, s.failures, s.lastLogged = false, 0, time.Time{}
}
