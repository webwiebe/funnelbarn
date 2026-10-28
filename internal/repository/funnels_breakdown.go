package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// MaxBreakdownGroups caps the groups of one breakdown; the rest collapse into
// one "other" group so the response stays bounded.
const MaxBreakdownGroups = 20

// FunnelBreakdownGroup is one value of the breakdown property and the funnel
// numbers of the entries that carry it.
type FunnelBreakdownGroup struct {
	// Value is the property value on the entry event. Empty for entries without
	// the property.
	Value string `json:"value"`
	// Label is what a UI shows: the value, "(none)" or "(other)".
	Label string `json:"label"`
	// Other marks the group that collects every value beyond the cap.
	Other bool `json:"other,omitempty"`
	// Entries is the number of funnel entries in the group (step 1 count).
	Entries int64              `json:"entries"`
	Results []FunnelStepResult `json:"results"`
}

// ValidBreakdownProperty reports whether name can be used as a breakdown
// property. Names go into a JSON path, so only a safe subset is accepted.
func ValidBreakdownProperty(name string) bool { return validPropertyName.MatchString(name) }

// funnelExtraWhere builds the WHERE fragment shared by every funnel query: the
// preset segment, stored segment rules and the scope's own requirements. The
// fragment starts with " AND " (or is empty) and uses the alias e for events.
func funnelExtraWhere(f Funnel, seg *SegmentFilter, rules []SegmentRule) (where string, args []any, needJoin bool) {
	presetClause, presetArg, needJoin := segmentParam(seg)
	ruleClause, ruleArgs, rulesNeedJoin := buildSegmentRuleClause(rules)
	needJoin = needJoin || rulesNeedJoin

	var clauses []string
	if presetClause != "" {
		clauses = append(clauses, presetClause)
		if presetArg != nil {
			args = append(args, presetArg)
		}
	}
	if ruleClause != "" {
		clauses = append(clauses, ruleClause)
		args = append(args, ruleArgs...)
	}
	switch f.Scope {
	case FunnelScopePageView:
		clauses = append(clauses, "e.page_view_id IS NOT NULL")
	case FunnelScopeUser:
		clauses = append(clauses, "e.user_id_hash IS NOT NULL AND e.user_id_hash != ''")
	}
	if len(clauses) > 0 {
		where = " AND " + strings.Join(clauses, " AND ")
	}
	return where, args, needJoin
}

// AnalyzeFunnelBreakdown runs the funnel like AnalyzeFunnel and splits it by
// the value property has on each entry's first step event. Every entry lands in
// exactly one group, so the group counts add up to the unbroken funnel. Groups
// come back ordered by entries (then value), capped at MaxBreakdownGroups.
func (s *ReadStore) AnalyzeFunnelBreakdown(ctx context.Context, f Funnel, from, to time.Time, seg *SegmentFilter, property string, rules ...SegmentRule) ([]FunnelBreakdownGroup, error) {
	if len(f.Steps) == 0 {
		return nil, nil
	}
	if !validPropertyName.MatchString(property) {
		return nil, fmt.Errorf("breakdown property %q: only letters, digits and underscore are allowed", property)
	}
	extraWhere, extraArgs, needJoin := funnelExtraWhere(f, seg, rules)

	var counts map[string][]int64
	var err error
	if f.Scope == FunnelScopeUser {
		counts, err = s.userBreakdownCounts(ctx, f, from, to, extraWhere, extraArgs, needJoin, property)
	} else {
		counts, err = s.entryBreakdownCounts(ctx, f, from, to, extraWhere, extraArgs, needJoin, property)
	}
	if err != nil {
		return nil, err
	}
	return breakdownGroups(f, counts), nil
}

// breakdownGroups orders the per-value step counts, caps them and turns them
// into groups with conversion and drop-off.
func breakdownGroups(f Funnel, counts map[string][]int64) []FunnelBreakdownGroup {
	values := make([]string, 0, len(counts))
	for v := range counts {
		values = append(values, v)
	}
	sort.Slice(values, func(i, j int) bool {
		a, b := counts[values[i]][0], counts[values[j]][0]
		if a != b {
			return a > b
		}
		return values[i] < values[j]
	})

	groups := make([]FunnelBreakdownGroup, 0, min(len(values), MaxBreakdownGroups+1))
	var other []int64
	for i, v := range values {
		if i >= MaxBreakdownGroups {
			if other == nil {
				other = make([]int64, len(f.Steps))
			}
			for k, n := range counts[v] {
				other[k] += n
			}
			continue
		}
		label := v
		if v == "" {
			label = "(none)"
		}
		groups = append(groups, FunnelBreakdownGroup{
			Value: v, Label: label, Entries: counts[v][0], Results: funnelResults(f, counts[v]),
		})
	}
	if other != nil {
		groups = append(groups, FunnelBreakdownGroup{
			Label: "(other)", Other: true, Entries: other[0], Results: funnelResults(f, other),
		})
	}
	return groups
}

// entryBreakdownCounts counts, per entry value and per step, the distinct
// sessions (or page views) that fired the step, among those whose first step
// event carries the value. Like the unbroken funnel, the steps are not ordered.
func (s *ReadStore) entryBreakdownCounts(ctx context.Context, f Funnel, from, to time.Time, extraWhere string, extraArgs []any, needJoin bool, property string) (map[string][]int64, error) {
	counts := map[string][]int64{}
	for i := range f.Steps {
		q, args := entryBreakdownStepSQL(f, i, from, to, extraWhere, extraArgs, needJoin, property)
		rows, err := s.rdb.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, fmt.Errorf("breakdown step %d: %w", i, err)
		}
		for rows.Next() {
			var v string
			var n int64
			if err := rows.Scan(&v, &n); err != nil {
				_ = rows.Close()
				return nil, err
			}
			if counts[v] == nil {
				counts[v] = make([]int64, len(f.Steps))
			}
			counts[v][i] = n
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return counts, nil
}

// entryBreakdownStepSQL is the query entryBreakdownCounts runs for step i: per
// entry value, the distinct sessions (or page views) that fired the step.
func entryBreakdownStepSQL(f Funnel, i int, from, to time.Time, extraWhere string, extraArgs []any, needJoin bool, property string) (string, []any) {
	key := "e.session_id"
	if f.Scope == FunnelScopePageView {
		key = "e.page_view_id"
	}
	join := ""
	if needJoin {
		join = " JOIN sessions s ON s.id = e.session_id AND s.project_id = e.project_id"
	}
	valueExpr := fmt.Sprintf("COALESCE(json_extract(e.properties, '$.%s'), '')", property)

	// The entry value is the one on the entry's earliest step 1 event (SQLite
	// returns the bare columns of the MIN row).
	entrySQL := fmt.Sprintf(`
		SELECT %[1]s AS k, %[2]s AS v, MIN(e.occurred_at)
		FROM events e%[3]s
		WHERE e.project_id = ? AND e.name = ? AND e.occurred_at >= ? AND e.occurred_at <= ?%[4]s`,
		key, valueExpr, join, extraWhere)
	entryArgs := append([]any{f.ProjectID, f.Steps[0].EventName, from, to}, extraArgs...)
	entrySQL, entryArgs = stepFilterClause(entrySQL, entryArgs, f.Steps[0])
	entrySQL += " GROUP BY " + key

	step := f.Steps[i]
	q := fmt.Sprintf(`
		WITH entry AS (%[1]s)
		SELECT entry.v, COUNT(DISTINCT %[2]s)
		FROM events e%[3]s
		JOIN entry ON entry.k = %[2]s
		WHERE e.project_id = ? AND e.name = ? AND e.occurred_at >= ? AND e.occurred_at <= ?%[4]s`,
		entrySQL, key, join, extraWhere)
	args := append(append([]any{}, entryArgs...), f.ProjectID, step.EventName, from, to)
	args = append(args, extraArgs...)
	q, args = stepFilterClause(q, args, step)
	return q + " GROUP BY entry.v", args
}

// userBreakdownCounts is the user-scope breakdown: each user is grouped by the
// property on their earliest step 1 event and counted through the same ordered
// steps as the unbroken funnel.
func (s *ReadStore) userBreakdownCounts(ctx context.Context, f Funnel, from, to time.Time, extraWhere string, extraArgs []any, needJoin bool, property string) (map[string][]int64, error) {
	depths, err := s.orderedUserDepths(ctx, f, from, to, extraWhere, extraArgs, needJoin)
	if err != nil {
		return nil, err
	}
	q, args := userBreakdownEntrySQL(f, from, to, extraWhere, extraArgs, needJoin, property)
	rows, err := s.rdb.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("breakdown entry values: %w", err)
	}
	defer func() { _ = rows.Close() }()
	counts := map[string][]int64{}
	for rows.Next() {
		var user, value string
		var firstAt any // MIN() loses the column type; only its row selection matters
		if err := rows.Scan(&user, &value, &firstAt); err != nil {
			return nil, err
		}
		depth, ok := depths[user]
		if !ok {
			continue
		}
		if counts[value] == nil {
			counts[value] = make([]int64, len(f.Steps))
		}
		for k := 0; k < depth; k++ {
			counts[value][k]++
		}
	}
	return counts, rows.Err()
}

// userBreakdownEntrySQL selects each user's property value from their earliest
// step 1 event.
func userBreakdownEntrySQL(f Funnel, from, to time.Time, extraWhere string, extraArgs []any, needJoin bool, property string) (string, []any) {
	join := ""
	if needJoin {
		join = " JOIN sessions s ON s.id = e.session_id AND s.project_id = e.project_id"
	}
	q := fmt.Sprintf(`
		SELECT e.user_id_hash, COALESCE(json_extract(e.properties, '$.%s'), ''), MIN(e.occurred_at)
		FROM events e%s
		WHERE e.project_id = ? AND e.name = ? AND e.occurred_at >= ? AND e.occurred_at <= ?%s`,
		property, join, extraWhere)
	args := append([]any{f.ProjectID, f.Steps[0].EventName, from, to}, extraArgs...)
	q, args = stepFilterClause(q, args, f.Steps[0])
	return q + " GROUP BY e.user_id_hash", args
}
