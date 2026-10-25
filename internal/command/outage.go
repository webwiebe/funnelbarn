package command

import (
	"log/slog"
	"sync"
	"time"
)

// DefaultOutageAfter is Outage.After when zero. Shared Valkey stalls for a
// few seconds several times a day; every caller has a fallback that loses
// nothing, so only a run of failures that lasts this long is an incident.
const DefaultOutageAfter = 30 * time.Second

// Outage tracks one run of failed Valkey calls and picks the level to log
// each failure at: Warn for the first, Error once when the run has lasted
// After, Debug for the rest. BugBarn opens an issue for every Error, so a
// stall the fallback absorbs stays out of it. The zero value is ready and it
// is safe for concurrent use.
type Outage struct {
	// After is how long failures must go on before one is logged at Error.
	// Zero means DefaultOutageAfter.
	After time.Duration
	// Now is the clock. Nil means time.Now.
	Now func() time.Time

	mu       sync.Mutex
	failing  bool
	reported bool
	start    time.Time
}

// Fail records a failure and returns the level to log it at.
func (o *Outage) Fail() slog.Level {
	o.mu.Lock()
	defer o.mu.Unlock()
	now := o.now()
	if !o.failing {
		o.failing = true
		o.reported = false
		o.start = now
		return slog.LevelWarn
	}
	if !o.reported && now.Sub(o.start) >= o.after() {
		o.reported = true
		return slog.LevelError
	}
	return slog.LevelDebug
}

// Recover ends a run of failures. It reports whether one was going on and how
// long it lasted.
func (o *Outage) Recover() (time.Duration, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.failing {
		return 0, false
	}
	o.failing = false
	return o.now().Sub(o.start), true
}

func (o *Outage) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func (o *Outage) after() time.Duration {
	if o.After > 0 {
		return o.After
	}
	return DefaultOutageAfter
}
