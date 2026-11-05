package volumedrop

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var testNow = time.Date(2026, 10, 30, 12, 30, 0, 0, time.UTC)

// steady returns perHour events in every hour Evaluate reads, up to but excluding
// the clock hour silentFrom. A zero silentFrom never goes silent.
func steady(perHour int64, silentFrom time.Time) map[string]int64 {
	counts := map[string]int64{}
	end := testNow.Truncate(time.Hour)
	for h := Since(testNow); h.Before(end); h = h.Add(time.Hour) {
		if silentFrom.IsZero() || h.Before(silentFrom) {
			counts[h.Format(HourFormat)] = perHour
		}
	}
	return counts
}

// A project sending 10 events an hour has a baseline of 240, so it is low under
// 24. Silent for 22 hours it has 20 left in the window.
func TestEvaluate_FiresWhenTheProjectBecomesLow(t *testing.T) {
	end := testNow.Truncate(time.Hour)
	hours := map[string]map[string]int64{"shop": steady(10, end.Add(-22*time.Hour))}

	started, open := Evaluate(hours, nil, testNow)

	require.Equal(t, []Drop{{Project: "shop", Baseline: 240, Current: 20}}, started)
	require.Equal(t, []string{"shop"}, open)
}

// One hour later the project is still low and its drop is already open.
func TestEvaluate_StaysQuietWhileTheDropIsOpen(t *testing.T) {
	end := testNow.Truncate(time.Hour)
	hours := map[string]map[string]int64{"shop": steady(10, end.Add(-23*time.Hour))}

	started, open := Evaluate(hours, []string{"shop"}, testNow)

	require.Empty(t, started)
	require.Equal(t, []string{"shop"}, open)
}

// The writer was down in the hour the project became low. The report comes late.
func TestEvaluate_FiresAfterAMissedHour(t *testing.T) {
	end := testNow.Truncate(time.Hour)
	hours := map[string]map[string]int64{"shop": steady(10, end.Add(-23*time.Hour))}

	started, open := Evaluate(hours, nil, testNow)

	require.Equal(t, []Drop{{Project: "shop", Baseline: 240, Current: 10}}, started)
	require.Equal(t, []string{"shop"}, open)
}

func TestEvaluate_QuietForAHealthyProject(t *testing.T) {
	hours := map[string]map[string]int64{"shop": steady(10, time.Time{})}

	started, open := Evaluate(hours, nil, testNow)

	require.Empty(t, started)
	require.Empty(t, open)
}

func TestEvaluate_ExactlyATenthIsNotLow(t *testing.T) {
	end := testNow.Truncate(time.Hour)
	counts := steady(10, end.Add(-24*time.Hour))
	counts[end.Add(-time.Hour).Format(HourFormat)] = 24

	started, open := Evaluate(map[string]map[string]int64{"shop": counts}, nil, testNow)

	require.Empty(t, started)
	require.Empty(t, open)
}

// With a baseline of 240 a project at 30 is over a tenth and under a fifth: its
// open drop stays open, and it starts none. At 48 it is back.
func TestEvaluate_ClosesAtAFifthOfTheBaseline(t *testing.T) {
	end := testNow.Truncate(time.Hour)
	counts := steady(10, end.Add(-24*time.Hour))
	hours := map[string]map[string]int64{"shop": counts}

	counts[end.Add(-time.Hour).Format(HourFormat)] = 30
	started, open := Evaluate(hours, []string{"shop"}, testNow)
	require.Empty(t, started)
	require.Equal(t, []string{"shop"}, open)

	started, open = Evaluate(hours, nil, testNow)
	require.Empty(t, started)
	require.Empty(t, open)

	counts[end.Add(-time.Hour).Format(HourFormat)] = 48
	started, open = Evaluate(hours, []string{"shop"}, testNow)
	require.Empty(t, started)
	require.Empty(t, open)
}

// A project under the floor of 50 events a day has no level to fall from.
func TestEvaluate_IgnoresAProjectUnderTheBaselineFloor(t *testing.T) {
	end := testNow.Truncate(time.Hour)
	counts := steady(2, end.Add(-22*time.Hour)) // baseline 48, current 4

	started, open := Evaluate(map[string]map[string]int64{"quiet": counts}, nil, testNow)

	require.Empty(t, started)
	require.Empty(t, open)
}

// A project that began sending three days ago has too few active days, however
// busy those days were.
func TestEvaluate_IgnoresAProjectWithoutSevenDaysOfHistory(t *testing.T) {
	end := testNow.Truncate(time.Hour)
	counts := steady(100, end.Add(-22*time.Hour))
	for h := Since(testNow); h.Before(end.Add(-3 * day)); h = h.Add(time.Hour) {
		delete(counts, h.Format(HourFormat))
	}

	started, open := Evaluate(map[string]map[string]int64{"new": counts}, nil, testNow)

	require.Empty(t, started)
	require.Empty(t, open)
}

// After 3 silent days the empty days leave too few active ones, so the project no
// longer counts as low. Its baseline holds, so the drop stays open and nothing fires.
func TestEvaluate_OpenDropOutlastsTheActiveDayRule(t *testing.T) {
	end := testNow.Truncate(time.Hour)
	hours := map[string]map[string]int64{"gone": steady(10, end.Add(-3*day))}

	started, open := Evaluate(hours, []string{"gone"}, testNow)

	require.Empty(t, started)
	require.Equal(t, []string{"gone"}, open)
}

// After 5 silent days the baseline itself is gone. The drop closes without a new
// report, and a closed project does not start a second one.
func TestEvaluate_LongSilenceClosesWithoutFiring(t *testing.T) {
	end := testNow.Truncate(time.Hour)
	hours := map[string]map[string]int64{"gone": steady(10, end.Add(-5*day))}

	started, open := Evaluate(hours, []string{"gone"}, testNow)
	require.Empty(t, started)
	require.Empty(t, open)

	started, open = Evaluate(hours, nil, testNow)
	require.Empty(t, started)
	require.Empty(t, open)
}

// A project with no rows left in the range cannot stay open.
func TestEvaluate_ForgetsAProjectWithoutRows(t *testing.T) {
	started, open := Evaluate(map[string]map[string]int64{}, []string{"deleted"}, testNow)

	require.Empty(t, started)
	require.Empty(t, open)
}

// Rows in the running hour must not change the answer: the hour is incomplete.
func TestEvaluate_IgnoresTheRunningHour(t *testing.T) {
	end := testNow.Truncate(time.Hour)
	counts := steady(10, end.Add(-22*time.Hour))
	counts[end.Format(HourFormat)] = 1000

	started, _ := Evaluate(map[string]map[string]int64{"shop": counts}, nil, testNow)

	require.Equal(t, []Drop{{Project: "shop", Baseline: 240, Current: 20}}, started)
}

func TestEvaluate_SortsByProject(t *testing.T) {
	end := testNow.Truncate(time.Hour)
	hours := map[string]map[string]int64{
		"b": steady(10, end.Add(-22*time.Hour)),
		"a": steady(10, end.Add(-22*time.Hour)),
	}

	started, open := Evaluate(hours, nil, testNow)

	require.Len(t, started, 2)
	require.Equal(t, "a", started[0].Project)
	require.Equal(t, "b", started[1].Project)
	require.Equal(t, []string{"a", "b"}, open)
}

// agents falls whenever the rig goes quiet, so its drop is no signal.
func TestEvaluate_LeavesOutExcludedProjects(t *testing.T) {
	end := testNow.Truncate(time.Hour)
	hours := map[string]map[string]int64{"agents": steady(10, end.Add(-22*time.Hour))}

	started, open := Evaluate(hours, nil, testNow)

	require.Empty(t, started)
	require.Empty(t, open)
}

// Since must reach the oldest hour of the oldest baseline day, or a long-lived
// project would look a day short of history.
func TestSince_CoversTheOldestBaselineDay(t *testing.T) {
	end := testNow.Truncate(time.Hour)
	require.Equal(t, end.Add(-7*day-window), Since(testNow))
}
