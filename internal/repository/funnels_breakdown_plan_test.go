package repository

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// queryPlan returns the EXPLAIN QUERY PLAN detail lines of q.
func queryPlan(t *testing.T, s *Store, q string, args []any) string {
	t.Helper()
	rows, err := s.DB().QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+q, args...)
	require.NoError(t, err)
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &notused, &detail))
		plan = append(plan, detail)
	}
	require.NoError(t, rows.Err())
	return strings.Join(plan, "\n")
}

// fullScan matches a plan line that scans events (e) or sessions (s) in full.
// The CTE (entry) may be scanned; it holds one row per funnel entry.
var fullScan = regexp.MustCompile(`(?m)SCAN (e|s)( |$)`)

// The breakdown reads events the way the unbroken funnel does: every access
// to events goes through an index on the project, event name and time window,
// whatever the scope and filters. A full scan of events would grow with the
// whole table instead of the funnel's window.
func TestAnalyzeFunnelBreakdown_QueryPlanSearchesEvents(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "plan.db"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(7 * 24 * time.Hour)
	steps := []FunnelStep{{EventName: "visit"}, {EventName: "signup"}}

	filters := map[string]struct {
		seg   *SegmentFilter
		rules []SegmentRule
	}{
		"none":              {},
		"environment":       {rules: EnvironmentRule("production")},
		"preset event":      {seg: &SegmentFilter{Field: "device_type", Op: "eq", Value: "mobile"}},
		"preset session":    {seg: &SegmentFilter{Field: "session_returning", Value: "true"}},
		"stored on session": {rules: []SegmentRule{{Field: "country_code", Operator: "eq", Value: "NL"}}},
	}
	for _, scope := range []string{FunnelScopeSession, FunnelScopePageView, FunnelScopeUser} {
		f := Funnel{ProjectID: "p", Scope: scope, Steps: steps}
		for name, filter := range filters {
			extraWhere, extraArgs, needJoin := funnelExtraWhere(f, filter.seg, filter.rules)
			var queries [][2]any
			if scope == FunnelScopeUser {
				q, args := userBreakdownEntrySQL(f, from, to, extraWhere, extraArgs, needJoin, "surface")
				queries = append(queries, [2]any{q, args})
			} else {
				for i := range steps {
					q, args := entryBreakdownStepSQL(f, i, from, to, extraWhere, extraArgs, needJoin, "surface")
					queries = append(queries, [2]any{q, args})
				}
			}
			for _, qa := range queries {
				plan := queryPlan(t, s, qa[0].(string), qa[1].([]any))
				where := scope + " / " + name + ":\n" + plan
				require.Contains(t, plan, "SEARCH e USING INDEX idx_events_project_occurred (project_id=? AND occurred_at>? AND occurred_at<?)", where)
				require.NotRegexp(t, fullScan, plan, where)
				if needJoin {
					require.Contains(t, plan, "SEARCH s USING INDEX", where)
				}
			}
		}
	}
}
