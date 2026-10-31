package command

import (
	"log/slog"
	"sync"
	"time"
)

const (
	// DefaultOutageAfter is Outage.After when zero. Shared Valkey stalls for a
	// few seconds several times a day; every caller has a fallback that loses
	// nothing, so only this much stall time inside OutageWindow is an incident.
	DefaultOutageAfter = 30 * time.Second
	// OutageWindow is how far back Outage adds up stall time, and how often it
	// logs at Error at most.
	OutageWindow = 10 * time.Minute
)

// Outage tracks runs of failed Valkey calls and picks the level to log each
// failure at: Error once per OutageWindow when the stall time in that window
// reaches After, Warn for the first failure of a run, Debug for the rest.
// BugBarn opens an issue for every Error, so a stall the fallback absorbs stays
// out of it, and repeated stalls with a success between them still reach it.
//
// A run counts from its first failed call to its last, and at least
// DefaultPublishTimeout, what one failed call can cost. A quiet period counts
// for nothing: a success after it ends the run at the last failed call, and a
// failure more than After after the previous one starts a new run. The zero
// value is ready and it is safe for concurrent use.
type Outage struct {
	// After is how much stall time inside OutageWindow is logged at Error.
	// Zero means DefaultOutageAfter.
	After time.Duration
	// Now is the clock. Nil means time.Now.
	Now func() time.Time

	mu         sync.Mutex
	failing    bool
	start      time.Time
	last       time.Time
	reportedAt time.Time
	closed     []outageRuns
}

// outageRuns is one or more ended runs. Runs that end within a second of each
// other share an entry, so a flapping Valkey adds at most one entry a second.
type outageRuns struct {
	end    time.Time
	lasted time.Duration
	count  int
}

// Fail records a failure. It returns the level to log it at and an attribute
// with the stall time and the number of runs in the window. The total is capped
// at OutageWindow, so a stall going on for hours reports 10m0s.
func (o *Outage) Fail() (slog.Level, slog.Attr) {
	o.mu.Lock()
	defer o.mu.Unlock()
	now := o.now()
	if o.failing && now.Sub(o.last) > o.after() {
		o.closeRun()
	}
	level := slog.LevelDebug
	if !o.failing {
		o.failing = true
		o.start = now
		level = slog.LevelWarn
	}
	o.last = now

	total, runs := o.inWindow(now)
	if total >= o.after() && (o.reportedAt.IsZero() || now.Sub(o.reportedAt) >= OutageWindow) {
		o.reportedAt = now
		level = slog.LevelError
	}
	return level, slog.Group("outage", "total", total.String(), "runs", runs)
}

// Recover ends a run of failures. It reports whether one was going on and how
// much stall time it counted for.
func (o *Outage) Recover() (time.Duration, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.failing {
		return 0, false
	}
	return o.closeRun(), true
}

// closeRun moves the current run to closed and returns its stall time.
func (o *Outage) closeRun() time.Duration {
	lasted := o.currentRun()
	o.failing = false
	if n := len(o.closed); n > 0 && o.last.Sub(o.closed[n-1].end) < time.Second {
		o.closed[n-1].end = o.last
		o.closed[n-1].lasted += lasted
		o.closed[n-1].count++
		return lasted
	}
	o.closed = append(o.closed, outageRuns{end: o.last, lasted: lasted, count: 1})
	return lasted
}

func (o *Outage) currentRun() time.Duration {
	return max(o.last.Sub(o.start), DefaultPublishTimeout)
}

// inWindow returns the stall time and the number of runs in the OutageWindow
// up to now, and drops closed runs that ended before it. The current run counts.
func (o *Outage) inWindow(now time.Time) (time.Duration, int) {
	cutoff := now.Add(-OutageWindow)
	kept := o.closed[:0]
	for _, r := range o.closed {
		if r.end.After(cutoff) {
			kept = append(kept, r)
		}
	}
	o.closed = kept

	total := min(o.currentRun(), OutageWindow)
	runs := 1
	for _, r := range o.closed {
		total += min(r.lasted, r.end.Sub(cutoff))
		runs += r.count
	}
	return min(total, OutageWindow), runs
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
