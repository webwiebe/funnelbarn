package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/wiebe-xyz/funnelbarn/internal/environment"
)

// environmentField is the events column the funnel environment argument filters
// on. It is deliberately absent from AllowedSegmentFields so a stored segment
// cannot name it; only EnvironmentRule produces a rule for it.
const environmentField = "environment"

// EnvironmentRule narrows a funnel analysis to one environment. env goes through
// the same normalisation as ingest ("prod" is "production"); an empty env means
// all environments and returns no rules. Events are matched by equality, like
// the event-count filters.
func EnvironmentRule(env string) []SegmentRule {
	if strings.TrimSpace(env) == "" {
		return nil
	}
	return []SegmentRule{{Field: environmentField, Operator: "eq", Value: environment.Normalize(env)}}
}

// SegmentFilter filters funnel analysis by a field condition on the events (or joined sessions) table.
type SegmentFilter struct {
	// Field is the column to filter on: "device_type", "user_id_hash", "browser",
	// "country_code", or the special value "session_returning".
	Field string
	// Op is the comparison operator: "eq", "neq", "is_null", "is_not_null".
	Op string
	// Value is the right-hand side for "eq" / "neq" operators.
	Value string
}

// segmentParam returns a SQL WHERE fragment with a placeholder and the value to bind,
// plus whether a sessions JOIN is needed.
// arg is nil when no bind parameter is needed for the clause (e.g. IS NULL checks).
func segmentParam(seg *SegmentFilter) (clause string, arg any, needSessionJoin bool) {
	if seg == nil {
		return "", nil, false
	}
	if seg.Field == "session_returning" {
		if seg.Value == "true" {
			return "s.event_count > 1", nil, true
		}
		return "s.event_count = 1", nil, true
	}

	// Whitelist allowed field names to prevent column injection.
	allowedFields := map[string]bool{
		"device_type":  true,
		"browser":      true,
		"os":           true,
		"country_code": true,
		"user_id_hash": true,
	}
	if !allowedFields[seg.Field] {
		return "", nil, false // silently ignore unknown fields
	}

	col := "e." + seg.Field
	switch seg.Op {
	case "eq":
		return col + " = ?", seg.Value, false
	case "neq":
		return col + " != ?", seg.Value, false
	case "is_null":
		return fmt.Sprintf("(%s IS NULL OR %s = '')", col, col), nil, false
	case "is_not_null":
		return fmt.Sprintf("(%s IS NOT NULL AND %s != '')", col, col), nil, false
	}
	return "", nil, false
}

// ruleTableAlias maps a rule field to its table alias. The environment field is
// only reachable through EnvironmentRule, so it is resolved here and not listed
// in AllowedSegmentFields.
func ruleTableAlias(field string) (string, bool) {
	if field == environmentField {
		return "e", true
	}
	alias, ok := AllowedSegmentFields[field]
	return alias, ok
}

// buildSegmentRuleClause converts stored segment rules into a SQL WHERE fragment.
func buildSegmentRuleClause(rules []SegmentRule) (clause string, args []any, needJoin bool) {
	if len(rules) == 0 {
		return "", nil, false
	}
	var parts []string
	for _, rule := range rules {
		tableAlias, ok := ruleTableAlias(rule.Field)
		if !ok {
			continue
		}
		if tableAlias == "s" {
			needJoin = true
		}
		col := tableAlias + "." + rule.Field
		switch rule.Operator {
		case "eq":
			parts = append(parts, col+" = ?")
			args = append(args, rule.Value)
		case "neq":
			parts = append(parts, col+" != ?")
			args = append(args, rule.Value)
		case "contains":
			parts = append(parts, col+" LIKE ?")
			args = append(args, "%"+rule.Value+"%")
		case "not_contains":
			parts = append(parts, col+" NOT LIKE ?")
			args = append(args, "%"+rule.Value+"%")
		case "is_null":
			parts = append(parts, fmt.Sprintf("(%s IS NULL OR %s = '')", col, col))
		case "is_not_null":
			parts = append(parts, fmt.Sprintf("(%s IS NOT NULL AND %s != '')", col, col))
		}
	}
	if len(parts) == 0 {
		return "", nil, false
	}
	return strings.Join(parts, " AND "), args, needJoin
}

// FunnelSegments holds distinct values available for dynamic segment filtering.
type FunnelSegments struct {
	DeviceTypes []string `json:"device_types"`
	Browsers    []string `json:"browsers"`
	Countries   []string `json:"countries"`
}

// FunnelSegmentData returns distinct field values present in the events for a project.
func (s *ReadStore) FunnelSegmentData(ctx context.Context, projectID string) (FunnelSegments, error) {
	var out FunnelSegments

	// Column names are interpolated into SQL, so guard against any caller ever
	// passing an unvetted value: only an explicit allowlist of event columns is
	// permitted. Callers below pass literals; this is defence-in-depth.
	allowedDistinctCols := map[string]bool{"device_type": true, "browser": true, "country_code": true}

	fetchDistinct := func(col string) ([]string, error) {
		if !allowedDistinctCols[col] {
			return nil, fmt.Errorf("FunnelSegmentData: disallowed column %q", col)
		}
		q := fmt.Sprintf(`SELECT DISTINCT %s FROM events WHERE project_id = ? AND %s IS NOT NULL AND %s != '' ORDER BY %s`, col, col, col, col)
		rows, err := s.rdb.QueryContext(ctx, q, projectID)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		var vals []string
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				return nil, err
			}
			vals = append(vals, v)
		}
		return vals, rows.Err()
	}

	var err error
	if out.DeviceTypes, err = fetchDistinct("device_type"); err != nil {
		return out, err
	}
	if out.Browsers, err = fetchDistinct("browser"); err != nil {
		return out, err
	}
	if out.Countries, err = fetchDistinct("country_code"); err != nil {
		return out, err
	}
	return out, nil
}
