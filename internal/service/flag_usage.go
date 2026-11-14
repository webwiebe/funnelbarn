package service

import (
	"context"
	"sort"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// FlagUsageWindowDays is the span of the usage report. The query is bounded to
// exactly this many UTC days, which keeps it on the project/time index.
const FlagUsageWindowDays = 30

// flagUsageCacheTTL keeps a polling dashboard from repeating the 30 day
// aggregate; usage moves slowly enough that a minute is invisible.
const flagUsageCacheTTL = time.Minute

const (
	// FlagUsageCountsNote says what the counts are. It is part of every
	// response so a reader of the API or MCP output sees it next to the numbers.
	FlagUsageCountsNote = "Counts are recorded evaluations: one row per evaluation of an experiment flag, " +
		"not sampled and not deduplicated per user or session. Config flags, dry runs (preview) and " +
		"disabled flags record no row, so their counts stay 0 and only last_evaluated_at shows use."
	// FlagUsageStaleNote is the exact meaning of the stale marker.
	FlagUsageStaleNote = "stale means no evaluations in the last 30 days. It says nothing about whether the flag is safe to delete."
)

// FlagUsage is the recent use of one flag.
type FlagUsage struct {
	FlagID          string           `json:"flag_id"`
	FlagKey         string           `json:"flag_key"`
	Name            string           `json:"name"`
	Kind            string           `json:"kind"`
	Status          string           `json:"status"`
	Evaluations7d   int64            `json:"evaluations_7d"`
	Evaluations30d  int64            `json:"evaluations_30d"`
	Daily           []int64          `json:"daily"`
	Variants30d     map[string]int64 `json:"variants_30d"`
	LastEvaluatedAt *time.Time       `json:"last_evaluated_at,omitempty"`
	Stale           bool             `json:"stale"`
}

// FlagUsageReport is the usage of every flag of a project.
type FlagUsageReport struct {
	WindowDays int         `json:"window_days"`
	CountsNote string      `json:"counts_note"`
	StaleNote  string      `json:"stale_note"`
	Flags      []FlagUsage `json:"flags"`
}

type flagUsageCacheEntry struct {
	report  FlagUsageReport
	expires time.Time
}

// Usage reports per flag the recorded evaluations of the last 30 UTC days (the
// current day included), split per day and variant, with the stale marker.
func (svc *FlagService) Usage(ctx context.Context, projectID string) (FlagUsageReport, error) {
	now := time.Now().UTC()
	svc.usageMu.Lock()
	if e, ok := svc.usageCache[projectID]; ok && now.Before(e.expires) {
		svc.usageMu.Unlock()
		return e.report, nil
	}
	svc.usageMu.Unlock()

	flags, err := svc.store.ListFlags(ctx, projectID)
	if err != nil {
		return FlagUsageReport{}, err
	}
	from, to := flagUsageWindow(now)
	rows, err := svc.store.FlagUsageDaily(ctx, projectID, from, to)
	if err != nil {
		return FlagUsageReport{}, err
	}
	report := buildFlagUsage(flags, rows, now)

	svc.usageMu.Lock()
	if svc.usageCache == nil || len(svc.usageCache) >= 256 {
		svc.usageCache = make(map[string]flagUsageCacheEntry)
	}
	svc.usageCache[projectID] = flagUsageCacheEntry{report: report, expires: now.Add(flagUsageCacheTTL)}
	svc.usageMu.Unlock()
	return report, nil
}

// flagUsageWindow returns [from, to): the start of the UTC day 29 days before
// now up to the start of tomorrow, exactly FlagUsageWindowDays days.
func flagUsageWindow(now time.Time) (from, to time.Time) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return today.AddDate(0, 0, -(FlagUsageWindowDays - 1)), today.AddDate(0, 0, 1)
}

// buildFlagUsage folds the per-day rows into one entry per flag. A flag counts
// as stale when it has no recorded evaluation in the window and was neither
// evaluated nor created in the last 30 days.
func buildFlagUsage(flags []domain.FeatureFlag, rows []domain.FlagUsageRow, now time.Time) FlagUsageReport {
	from, _ := flagUsageWindow(now)
	byFlag := make(map[string][]domain.FlagUsageRow, len(flags))
	for _, r := range rows {
		byFlag[r.FlagID] = append(byFlag[r.FlagID], r)
	}
	staleBefore := now.AddDate(0, 0, -FlagUsageWindowDays)

	out := make([]FlagUsage, 0, len(flags))
	for _, f := range flags {
		u := FlagUsage{
			FlagID: f.ID, FlagKey: f.FlagKey, Name: f.Name, Kind: f.Kind, Status: f.Status,
			Daily:           make([]int64, FlagUsageWindowDays),
			Variants30d:     map[string]int64{},
			LastEvaluatedAt: f.LastEvaluatedAt,
		}
		for _, r := range byFlag[f.ID] {
			day, err := time.Parse(time.DateOnly, r.Day)
			if err != nil {
				continue
			}
			idx := int(day.Sub(from).Hours() / 24)
			if idx < 0 || idx >= FlagUsageWindowDays {
				continue
			}
			u.Daily[idx] += r.Count
			u.Evaluations30d += r.Count
			if idx >= FlagUsageWindowDays-7 {
				u.Evaluations7d += r.Count
			}
			u.Variants30d[r.Variant] += r.Count
		}
		lastSeen := f.CreatedAt
		if f.LastEvaluatedAt != nil {
			lastSeen = *f.LastEvaluatedAt
		}
		u.Stale = u.Evaluations30d == 0 && lastSeen.Before(staleBefore)
		out = append(out, u)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Evaluations30d != out[j].Evaluations30d {
			return out[i].Evaluations30d > out[j].Evaluations30d
		}
		return out[i].FlagKey < out[j].FlagKey
	})
	return FlagUsageReport{
		WindowDays: FlagUsageWindowDays,
		CountsNote: FlagUsageCountsNote,
		StaleNote:  FlagUsageStaleNote,
		Flags:      out,
	}
}
