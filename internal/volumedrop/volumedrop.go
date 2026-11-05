// Package volumedrop finds projects whose events fell far under their own recent
// baseline. It holds no state: the caller passes the projects whose drop is open
// and keeps what Evaluate returns, so each drop is reported once.
//
// The thresholds come from a backtest over 21 days of production rows on
// 2026-10-30. By stored time: 5 episodes across 4 projects, including the drop of
// that morning. Shorter windows, counted by occurred_at, fired 19 to 25 times
// before leaving out agents, mostly on projects with quiet stretches.
package volumedrop

import (
	"sort"
	"time"
)

const (
	// HourFormat is the layout of the hour keys Detect reads, which is the first
	// 13 characters of a stored created_at.
	HourFormat = "2006-01-02 15"

	window = 24 * time.Hour
	day    = 24 * time.Hour

	baselineDays = 7

	// minActiveDays and minBaseline keep out a project without a real baseline.
	// A project that started sending last week, or sends a handful of events a
	// day, has no level to fall from.
	minActiveDays = 6
	minBaseline   = 50

	// dropDivisor sets low at under a tenth of the baseline.
	dropDivisor = 10

	// recoverDivisor closes a drop once a project is back at a fifth of its
	// baseline. The gap keeps a project hovering around a tenth inside one drop.
	recoverDivisor = 5
)

// excluded names projects whose volume falls for a known cause. agents counts
// agent sessions, so it falls whenever the rig goes quiet.
var excluded = map[string]bool{"agents": true}

// Drop is a project whose drop starts in the hour Evaluate evaluated.
type Drop struct {
	Project  string
	Baseline int64 // median events in the same 24 hours over the prior 7 days
	Current  int64 // events in the last 24 hours
}

// Since returns the oldest hour Evaluate reads when evaluating at now.
func Since(now time.Time) time.Time {
	return now.UTC().Truncate(time.Hour).Add(-baselineDays*day - window)
}

// Evaluate returns the drops that start at now's clock hour, sorted by project,
// and the sorted projects whose drop is open afterwards. open lists the projects
// whose drop was open before. A low project outside open starts a drop, so an hour
// the writer missed delays the report and does not lose it. hours maps a project
// to its event count per HourFormat key. Only completed hours count, so the running
// hour's partial total lowers nothing.
func Evaluate(hours map[string]map[string]int64, open []string, now time.Time) (started []Drop, stillOpen []string) {
	end := now.UTC().Truncate(time.Hour)
	wasOpen := make(map[string]bool, len(open))
	for _, project := range open {
		wasOpen[project] = true
	}
	for project, counts := range hours {
		if excluded[project] {
			continue
		}
		baseline, current, low := evaluate(counts, end)
		if low && !wasOpen[project] {
			started = append(started, Drop{Project: project, Baseline: baseline, Current: current})
		}
		if low || (wasOpen[project] && current*recoverDivisor < baseline) {
			stillOpen = append(stillOpen, project)
		}
	}
	sort.Slice(started, func(i, j int) bool { return started[i].Project < started[j].Project })
	sort.Strings(stillOpen)
	return started, stillOpen
}

// evaluate compares the 24 hours before end with the same 24 hours on each of
// the prior 7 days.
func evaluate(counts map[string]int64, end time.Time) (baseline, current int64, low bool) {
	days := make([]int64, baselineDays)
	active := 0
	for d := range days {
		days[d] = total(counts, end.Add(-time.Duration(d+1)*day))
		if days[d] > 0 {
			active++
		}
	}
	sort.Slice(days, func(i, j int) bool { return days[i] < days[j] })
	baseline = days[baselineDays/2]
	current = total(counts, end)
	low = active >= minActiveDays && baseline >= minBaseline && current*dropDivisor < baseline
	return baseline, current, low
}

// total sums the window of hours ending just before end.
func total(counts map[string]int64, end time.Time) int64 {
	var n int64
	for h := end.Add(-window); h.Before(end); h = h.Add(time.Hour) {
		n += counts[h.Format(HourFormat)]
	}
	return n
}
