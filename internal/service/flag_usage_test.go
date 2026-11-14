package service

import (
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

func TestFlagUsageWindowIsExactlyThirtyUTCDays(t *testing.T) {
	now := time.Date(2026, 10, 15, 13, 45, 0, 0, time.UTC)
	from, to := flagUsageWindow(now)
	if want := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC); !from.Equal(want) {
		t.Errorf("from = %s, want %s", from, want)
	}
	if want := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC); !to.Equal(want) {
		t.Errorf("to = %s, want %s", to, want)
	}
	if got := to.Sub(from); got != 30*24*time.Hour {
		t.Errorf("window = %s, want 720h", got)
	}
}

func TestBuildFlagUsageFoldsRowsAndMarksStale(t *testing.T) {
	now := time.Date(2026, 10, 15, 13, 45, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -60)
	recent := now.AddDate(0, 0, -3)

	flags := []domain.FeatureFlag{
		{ID: "busy", FlagKey: "busy", Kind: "experiment", Status: "active", CreatedAt: old},
		{ID: "quiet", FlagKey: "quiet", Kind: "experiment", Status: "active", CreatedAt: old},
		{ID: "cfg", FlagKey: "cfg", Kind: "config", Status: "active", CreatedAt: old, LastEvaluatedAt: &recent},
		{ID: "fresh", FlagKey: "fresh", Kind: "experiment", Status: "active", CreatedAt: recent},
		{ID: "gone", FlagKey: "gone", Kind: "experiment", Status: "active", CreatedAt: old, LastEvaluatedAt: &old},
	}
	rows := []domain.FlagUsageRow{
		{FlagID: "busy", Day: "2026-10-15", Variant: "on", Count: 5},  // today: last daily slot, in 7d
		{FlagID: "busy", Day: "2026-10-09", Variant: "off", Count: 2}, // 6 days ago: still in 7d
		{FlagID: "busy", Day: "2026-10-08", Variant: "on", Count: 4},  // 7 days ago: 30d only
		{FlagID: "busy", Day: "2026-09-16", Variant: "on", Count: 1},  // first day of the window
		{FlagID: "busy", Day: "2026-09-15", Variant: "on", Count: 99}, // outside the window: ignored
		{FlagID: "busy", Day: "garbage", Variant: "on", Count: 99},    // unparsable: ignored
	}

	report := buildFlagUsage(flags, rows, now)

	if report.WindowDays != 30 || report.CountsNote == "" || report.StaleNote == "" {
		t.Errorf("report header = %+v, want window 30 with both notes", report)
	}
	byKey := map[string]FlagUsage{}
	for _, u := range report.Flags {
		byKey[u.FlagKey] = u
	}

	busy := byKey["busy"]
	if busy.Evaluations7d != 7 || busy.Evaluations30d != 12 {
		t.Errorf("busy 7d/30d = %d/%d, want 7/12", busy.Evaluations7d, busy.Evaluations30d)
	}
	if len(busy.Daily) != 30 || busy.Daily[29] != 5 || busy.Daily[23] != 2 || busy.Daily[22] != 4 || busy.Daily[0] != 1 {
		t.Errorf("busy daily = %v, want today 5 at [29], 2 at [23], 4 at [22], 1 at [0]", busy.Daily)
	}
	if busy.Variants30d["on"] != 10 || busy.Variants30d["off"] != 2 {
		t.Errorf("busy variants = %v, want on 10, off 2", busy.Variants30d)
	}
	if busy.Stale {
		t.Error("a flag with evaluations in the window is not stale")
	}

	if !byKey["quiet"].Stale {
		t.Error("an old flag that was never evaluated and has no rows is stale")
	}
	if byKey["cfg"].Stale {
		t.Error("a config flag evaluated 3 days ago (last_evaluated_at) is not stale although it records no rows")
	}
	if byKey["fresh"].Stale {
		t.Error("a flag created 3 days ago and never evaluated is not stale yet")
	}
	if !byKey["gone"].Stale {
		t.Error("a flag last evaluated 60 days ago with no rows is stale")
	}

	if report.Flags[0].FlagKey != "busy" {
		t.Errorf("first flag = %q, want the most used one (busy)", report.Flags[0].FlagKey)
	}
}
