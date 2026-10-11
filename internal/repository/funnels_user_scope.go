package repository

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// Funnel scopes: what one funnel entry is keyed on.
const (
	// FunnelScopeSession counts distinct sessions per step (the default).
	FunnelScopeSession = "session"
	// FunnelScopePageView counts distinct page views per step.
	FunnelScopePageView = "page_view"
	// FunnelScopeUser counts distinct users (user_id_hash) through ordered
	// steps: a user reaches step N only by firing it at or after their step
	// N-1, and within the funnel's conversion window of their step 1.
	FunnelScopeUser = "user"
)

// DefaultConversionWindow is the window of a user-scoped funnel whose
// ConversionWindowSeconds is unset.
const DefaultConversionWindow = 30 * 24 * time.Hour

// ConversionWindow returns the window of a user-scoped funnel.
func (f Funnel) ConversionWindow() time.Duration {
	if f.ConversionWindowSeconds == nil {
		return DefaultConversionWindow
	}
	return time.Duration(*f.ConversionWindowSeconds) * time.Second
}

// orderedUserStepCounts counts, per step, the users who reached it in order:
// step 1 in [from, to], and each later step in [from, to], at or after the
// user's previous step and within the conversion window of their step 1. A
// user with several step-1 events counts at the furthest step any of them
// reaches.
func (s *ReadStore) orderedUserStepCounts(ctx context.Context, f Funnel, from, to time.Time, extraWhere string, extraArgs []any, needJoin bool) ([]int64, error) {
	join := ""
	if needJoin {
		join = " JOIN sessions s ON s.id = e.session_id AND s.project_id = e.project_id"
	}
	window := f.ConversionWindow()

	// times[k][user] holds the user's step-k event times, ascending.
	times := make([]map[string][]time.Time, len(f.Steps))
	for i, step := range f.Steps {
		q := fmt.Sprintf(`
			SELECT e.user_id_hash, e.occurred_at
			FROM events e%s
			WHERE e.project_id = ? AND e.name = ? AND e.occurred_at >= ? AND e.occurred_at <= ?%s`,
			join, extraWhere)
		args := append([]any{f.ProjectID, step.EventName, from, to}, extraArgs...)
		q, args = stepFilterClause(q, args, step)
		q += " ORDER BY e.occurred_at"

		byUser, err := s.userEventTimes(ctx, q, args, times, i)
		if err != nil {
			return nil, fmt.Errorf("analyze step %d: %w", i, err)
		}
		times[i] = byUser
	}

	counts := make([]int64, len(f.Steps))
	for user, starts := range times[0] {
		best := 1
		for _, start := range starts {
			deadline := start.Add(window)
			depth, prev := 1, start
			for k := 1; k < len(f.Steps); k++ {
				ts := times[k][user]
				j := sort.Search(len(ts), func(n int) bool { return !ts[n].Before(prev) })
				if j == len(ts) || ts[j].After(deadline) {
					break
				}
				depth, prev = depth+1, ts[j]
			}
			best = max(best, depth)
			if best == len(f.Steps) {
				break
			}
		}
		for k := 0; k < best; k++ {
			counts[k]++
		}
	}
	return counts, nil
}

// userEventTimes runs one step query of orderedUserStepCounts. Past step 1 it
// keeps only users who have a step 1, since nobody else can reach the step.
func (s *ReadStore) userEventTimes(ctx context.Context, q string, args []any, times []map[string][]time.Time, step int) (map[string][]time.Time, error) {
	rows, err := s.rdb.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	byUser := map[string][]time.Time{}
	for rows.Next() {
		var user string
		var at time.Time
		if err := rows.Scan(&user, &at); err != nil {
			return nil, err
		}
		if step > 0 && times[0][user] == nil {
			continue
		}
		byUser[user] = append(byUser[user], at)
	}
	return byUser, rows.Err()
}
