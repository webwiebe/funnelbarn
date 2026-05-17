package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// validPropertyName matches only alphanumeric and underscore characters.
var validPropertyName = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// Funnel is a multi-step conversion funnel.
type Funnel struct {
	ID          string       `json:"id"`
	ProjectID   string       `json:"project_id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Scope       string       `json:"scope"` // "session" (default) or "page_view"
	Steps       []FunnelStep `json:"steps"`
	CreatedAt   time.Time    `json:"created_at"`
}

// FunnelStep is one step in a funnel.
type FunnelStep struct {
	ID        string         `json:"id"`
	FunnelID  string         `json:"funnel_id"`
	StepOrder int            `json:"step_order"`
	EventName string         `json:"event_name"`
	Filters   []FunnelFilter `json:"filters"`
}

// FunnelFilter filters events at a funnel step by property value.
type FunnelFilter struct {
	Property string `json:"property"`
	Value    string `json:"value"`
}

// FunnelStepResult holds analysis results for one funnel step.
type FunnelStepResult struct {
	StepOrder  int     `json:"step_order"`
	EventName  string  `json:"event_name"`
	Count      int64   `json:"count"`
	Conversion float64 `json:"conversion"`
	DropOff    float64 `json:"drop_off"`
}

// CreateFunnel inserts a funnel with its steps.
func (s *Store) CreateFunnel(ctx context.Context, f Funnel) (Funnel, error) {
	var err error
	f.ID, err = generateUUID()
	if err != nil {
		return Funnel{}, fmt.Errorf("generate uuid: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Funnel{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	if f.Scope == "" {
		f.Scope = "session"
	}
	const qf = `INSERT INTO funnels (id, project_id, name, description, scope) VALUES (?, ?, ?, ?, ?)`
	if _, err := tx.ExecContext(ctx, qf, f.ID, f.ProjectID, f.Name, nullStr(f.Description), f.Scope); err != nil {
		return Funnel{}, fmt.Errorf("insert funnel: %w", err)
	}

	for i, step := range f.Steps {
		if strings.TrimSpace(step.EventName) == "" {
			return Funnel{}, fmt.Errorf("step %d: event_name is required", i+1)
		}
	}

	const qs = `INSERT INTO funnel_steps (id, funnel_id, step_order, event_name, filters) VALUES (?, ?, ?, ?, ?)`
	for i := range f.Steps {
		f.Steps[i].ID, err = generateUUID()
		if err != nil {
			return Funnel{}, fmt.Errorf("generate uuid: %w", err)
		}
		f.Steps[i].FunnelID = f.ID
		f.Steps[i].StepOrder = i + 1

		filtersJSON, _ := json.Marshal(f.Steps[i].Filters)
		if _, err := tx.ExecContext(ctx, qs, f.Steps[i].ID, f.ID, f.Steps[i].StepOrder, f.Steps[i].EventName, string(filtersJSON)); err != nil {
			return Funnel{}, fmt.Errorf("insert funnel step: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Funnel{}, err
	}

	return s.FunnelByID(ctx, f.ID)
}

// FunnelByID fetches a funnel with all its steps.
func (s *Store) FunnelByID(ctx context.Context, id string) (Funnel, error) {
	const qf = `SELECT id, project_id, name, COALESCE(description,''), COALESCE(scope,'session'), created_at FROM funnels WHERE id = ?`
	var f Funnel
	if err := s.db.QueryRowContext(ctx, qf, id).Scan(&f.ID, &f.ProjectID, &f.Name, &f.Description, &f.Scope, &f.CreatedAt); err != nil {
		return Funnel{}, err
	}

	steps, err := s.funnelSteps(ctx, id)
	if err != nil {
		return Funnel{}, err
	}
	f.Steps = steps
	return f, nil
}

// ListFunnels returns all funnels for a project.
func (s *Store) ListFunnels(ctx context.Context, projectID string) ([]Funnel, error) {
	const q = `SELECT id, project_id, name, COALESCE(description,''), COALESCE(scope,'session'), created_at FROM funnels WHERE project_id = ? ORDER BY created_at`
	rows, err := s.db.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var funnels []Funnel
	for rows.Next() {
		var f Funnel
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Name, &f.Description, &f.Scope, &f.CreatedAt); err != nil {
			return nil, err
		}
		funnels = append(funnels, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range funnels {
		steps, err := s.funnelSteps(ctx, funnels[i].ID)
		if err != nil {
			return nil, err
		}
		funnels[i].Steps = steps
	}

	return funnels, nil
}

// UpdateFunnel replaces a funnel's name and steps.
func (s *Store) UpdateFunnel(ctx context.Context, f Funnel) (Funnel, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Funnel{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	if f.Scope == "" {
		f.Scope = "session"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE funnels SET name=?, description=?, scope=? WHERE id=?`, f.Name, nullStr(f.Description), f.Scope, f.ID); err != nil {
		return Funnel{}, fmt.Errorf("update funnel: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM funnel_steps WHERE funnel_id=?`, f.ID); err != nil {
		return Funnel{}, fmt.Errorf("delete funnel steps: %w", err)
	}

	const qs = `INSERT INTO funnel_steps (id, funnel_id, step_order, event_name, filters) VALUES (?, ?, ?, ?, ?)`
	for i := range f.Steps {
		f.Steps[i].ID, err = generateUUID()
		if err != nil {
			return Funnel{}, fmt.Errorf("generate uuid: %w", err)
		}
		f.Steps[i].FunnelID = f.ID
		f.Steps[i].StepOrder = i + 1

		filtersJSON, _ := json.Marshal(f.Steps[i].Filters)
		if _, err := tx.ExecContext(ctx, qs, f.Steps[i].ID, f.ID, f.Steps[i].StepOrder, f.Steps[i].EventName, string(filtersJSON)); err != nil {
			return Funnel{}, fmt.Errorf("insert funnel step: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Funnel{}, err
	}

	return s.FunnelByID(ctx, f.ID)
}

// DeleteFunnel removes a funnel and its steps (cascade).
func (s *Store) DeleteFunnel(ctx context.Context, id string) error {
	const q = `DELETE FROM funnels WHERE id = ?`
	_, err := s.db.ExecContext(ctx, q, id)
	return err
}

// funnelSteps returns steps for a funnel ordered by step_order.
func (s *Store) funnelSteps(ctx context.Context, funnelID string) ([]FunnelStep, error) {
	const q = `SELECT id, funnel_id, step_order, event_name, COALESCE(filters,'[]') FROM funnel_steps WHERE funnel_id = ? ORDER BY step_order`
	rows, err := s.db.QueryContext(ctx, q, funnelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []FunnelStep
	for rows.Next() {
		var step FunnelStep
		var filtersJSON string
		if err := rows.Scan(&step.ID, &step.FunnelID, &step.StepOrder, &step.EventName, &filtersJSON); err != nil {
			return nil, err
		}
		if filtersJSON != "" && filtersJSON != "[]" {
			_ = json.Unmarshal([]byte(filtersJSON), &step.Filters)
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
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

// AnalyzeFunnel computes conversion rates for each step of a funnel over a time range.
// seg is the legacy preset filter; rules are additional stored segment conditions (ANDed together).
func (s *Store) AnalyzeFunnel(ctx context.Context, f Funnel, from, to time.Time, seg *SegmentFilter, rules ...SegmentRule) ([]FunnelStepResult, error) {
	if len(f.Steps) == 0 {
		return nil, nil
	}

	scope := f.Scope
	if scope == "" {
		scope = "session"
	}
	pageViewScope := scope == "page_view"

	// Build WHERE clause from preset segment filter.
	presetClause, presetArg, needJoin := segmentParam(seg)

	// Build WHERE clause from stored segment rules.
	ruleClause, ruleArgs, rulesNeedJoin := buildSegmentRuleClause(rules)
	needJoin = needJoin || rulesNeedJoin

	// Combine clauses.
	var extraClauses []string
	var extraArgs []any
	if presetClause != "" {
		extraClauses = append(extraClauses, presetClause)
		if presetArg != nil {
			extraArgs = append(extraArgs, presetArg)
		}
	}
	if ruleClause != "" {
		extraClauses = append(extraClauses, ruleClause)
		extraArgs = append(extraArgs, ruleArgs...)
	}
	if pageViewScope {
		extraClauses = append(extraClauses, "e.page_view_id IS NOT NULL")
	}

	extraWhere := ""
	if len(extraClauses) > 0 {
		extraWhere = " AND " + strings.Join(extraClauses, " AND ")
	}

	// Choose the distinct column based on scope.
	distinctCol := "e.session_id"
	if pageViewScope {
		distinctCol = "e.page_view_id"
	}

	stepCounts := make([]int64, len(f.Steps))
	for i, step := range f.Steps {
		var n int64
		var q string
		var args []any

		if needJoin {
			q = fmt.Sprintf(`
				SELECT COUNT(DISTINCT %s)
				FROM events e
				JOIN sessions s ON s.id = e.session_id
				WHERE e.project_id = ? AND e.name = ? AND e.occurred_at >= ? AND e.occurred_at <= ?%s`,
				distinctCol, extraWhere)
		} else {
			q = fmt.Sprintf(`
				SELECT COUNT(DISTINCT %s)
				FROM events e
				WHERE e.project_id = ? AND e.name = ? AND e.occurred_at >= ? AND e.occurred_at <= ?%s`,
				distinctCol, extraWhere)
		}
		args = append([]any{f.ProjectID, step.EventName, from, to}, extraArgs...)

		// Step-level property filters.
		for _, filter := range step.Filters {
			if !validPropertyName.MatchString(filter.Property) {
				continue
			}
			q += fmt.Sprintf(" AND json_extract(e.properties, '$.%s') = ?", filter.Property)
			args = append(args, filter.Value)
		}

		if err := s.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
			return nil, fmt.Errorf("analyze step %d: %w", i, err)
		}
		stepCounts[i] = n
	}

	results := make([]FunnelStepResult, len(f.Steps))
	entry := stepCounts[0]
	for i, step := range f.Steps {
		r := FunnelStepResult{StepOrder: step.StepOrder, EventName: step.EventName, Count: stepCounts[i]}
		if entry > 0 {
			r.Conversion = float64(stepCounts[i]) / float64(entry)
		}
		if i > 0 && stepCounts[i-1] > 0 {
			r.DropOff = 1.0 - float64(stepCounts[i])/float64(stepCounts[i-1])
		}
		results[i] = r
	}
	return results, nil
}

// buildSegmentRuleClause converts stored segment rules into a SQL WHERE fragment.
func buildSegmentRuleClause(rules []SegmentRule) (clause string, args []any, needJoin bool) {
	if len(rules) == 0 {
		return "", nil, false
	}
	sessionFields := map[string]bool{
		"country_code": true, "city": true, "connection_class": true,
		"dark_mode": true, "browser_timezone": true,
	}
	var parts []string
	for _, rule := range rules {
		tableAlias, ok := AllowedSegmentFields[rule.Field]
		if !ok {
			continue
		}
		if tableAlias == "s" {
			needJoin = true
		}
		col := tableAlias + "." + rule.Field
		_ = sessionFields // already handled via allowedSegmentFields
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
func (s *Store) FunnelSegmentData(ctx context.Context, projectID string) (FunnelSegments, error) {
	var out FunnelSegments

	fetchDistinct := func(col string) ([]string, error) {
		q := fmt.Sprintf(`SELECT DISTINCT %s FROM events WHERE project_id = ? AND %s IS NOT NULL AND %s != '' ORDER BY %s`, col, col, col, col)
		rows, err := s.db.QueryContext(ctx, q, projectID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
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
