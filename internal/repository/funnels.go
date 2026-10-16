package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
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
	Scope       string       `json:"scope"` // "session" (default), "page_view" or "user"
	Steps       []FunnelStep `json:"steps"`
	CreatedAt   time.Time    `json:"created_at"`

	// ConversionWindowSeconds bounds a "user" funnel: every later step must
	// happen within this many seconds of step 1. Nil means
	// DefaultConversionWindow. The other scopes ignore it.
	ConversionWindowSeconds *int64 `json:"conversion_window_seconds,omitempty"`

	// UnmatchedSteps lists the step event names this project has never emitted.
	// A funnel whose steps reference names the project does not send can never
	// report a conversion, and presents as a legitimate zero-conversion funnel
	// rather than as misconfiguration — 20 of 37 funnels in production are in
	// that state. Read-only: computed on load, never persisted, ignored on write.
	UnmatchedSteps []string `json:"unmatched_steps,omitempty"`
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
	const qf = `INSERT INTO funnels (id, project_id, name, description, scope, conversion_window_seconds) VALUES (?, ?, ?, ?, ?, ?)`
	if _, err := tx.ExecContext(ctx, qf, f.ID, f.ProjectID, f.Name, nullStr(f.Description), f.Scope, nullInt64(f.ConversionWindowSeconds)); err != nil {
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

	return s.funnelByID(ctx, s.db, f.ID)
}

// FunnelByID fetches a funnel with all its steps.
func (s *ReadStore) FunnelByID(ctx context.Context, id string) (Funnel, error) {
	return s.funnelByID(ctx, s.rdb, id)
}

// funnelByID reads a funnel through db, so write paths can read back on the write pool.
func (s *ReadStore) funnelByID(ctx context.Context, db querier, id string) (Funnel, error) {
	const qf = `SELECT id, project_id, name, COALESCE(description,''), COALESCE(scope,'session'), created_at, conversion_window_seconds FROM funnels WHERE id = ?`
	var f Funnel
	var window sql.NullInt64
	if err := db.QueryRowContext(ctx, qf, id).Scan(&f.ID, &f.ProjectID, &f.Name, &f.Description, &f.Scope, &f.CreatedAt, &window); err != nil {
		return Funnel{}, err
	}
	f.ConversionWindowSeconds = int64Ptr(window)

	steps, err := s.funnelSteps(ctx, db, id)
	if err != nil {
		return Funnel{}, err
	}
	f.Steps = steps

	one := []Funnel{f}
	if err := s.annotateUnmatchedSteps(ctx, db, f.ProjectID, one); err != nil {
		return Funnel{}, err
	}
	return one[0], nil
}

// ListFunnels returns all funnels for a project.
func (s *ReadStore) ListFunnels(ctx context.Context, projectID string) ([]Funnel, error) {
	const q = `SELECT id, project_id, name, COALESCE(description,''), COALESCE(scope,'session'), created_at, conversion_window_seconds FROM funnels WHERE project_id = ? ORDER BY created_at`
	rows, err := s.rdb.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var funnels []Funnel
	for rows.Next() {
		var f Funnel
		var window sql.NullInt64
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Name, &f.Description, &f.Scope, &f.CreatedAt, &window); err != nil {
			return nil, err
		}
		f.ConversionWindowSeconds = int64Ptr(window)
		funnels = append(funnels, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range funnels {
		steps, err := s.funnelSteps(ctx, s.rdb, funnels[i].ID)
		if err != nil {
			return nil, err
		}
		funnels[i].Steps = steps
	}

	if err := s.annotateUnmatchedSteps(ctx, s.rdb, projectID, funnels); err != nil {
		return nil, err
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
	if _, err := tx.ExecContext(ctx, `UPDATE funnels SET name=?, description=?, scope=?, conversion_window_seconds=? WHERE id=?`, f.Name, nullStr(f.Description), f.Scope, nullInt64(f.ConversionWindowSeconds), f.ID); err != nil {
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

	return s.funnelByID(ctx, s.db, f.ID)
}

// DeleteFunnel removes a funnel and its steps (cascade).
func (s *Store) DeleteFunnel(ctx context.Context, id string) error {
	const q = `DELETE FROM funnels WHERE id = ?`
	_, err := s.db.ExecContext(ctx, q, id)
	return err
}

// funnelSteps returns steps for a funnel ordered by step_order.
func (s *ReadStore) funnelSteps(ctx context.Context, db querier, funnelID string) ([]FunnelStep, error) {
	const q = `SELECT id, funnel_id, step_order, event_name, COALESCE(filters,'[]') FROM funnel_steps WHERE funnel_id = ? ORDER BY step_order`
	rows, err := db.QueryContext(ctx, q, funnelID)
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
			if err := json.Unmarshal([]byte(filtersJSON), &step.Filters); err != nil {
				slog.WarnContext(ctx, "funnel step: malformed filters JSON", "step_id", step.ID, "raw_len", len(filtersJSON), "error", err)
			}
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

// AnalyzeFunnel computes conversion rates for each step of a funnel over a time range.
// seg is the legacy preset filter; rules are additional stored segment conditions (ANDed together).
func (s *ReadStore) AnalyzeFunnel(ctx context.Context, f Funnel, from, to time.Time, seg *SegmentFilter, rules ...SegmentRule) ([]FunnelStepResult, error) {
	if len(f.Steps) == 0 {
		return nil, nil
	}

	scope := f.Scope
	if scope == "" {
		scope = "session"
	}
	pageViewScope := scope == FunnelScopePageView
	userScope := scope == FunnelScopeUser

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
	if userScope {
		extraClauses = append(extraClauses, "e.user_id_hash IS NOT NULL AND e.user_id_hash != ''")
	}

	extraWhere := ""
	if len(extraClauses) > 0 {
		extraWhere = " AND " + strings.Join(extraClauses, " AND ")
	}

	if userScope {
		counts, err := s.orderedUserStepCounts(ctx, f, from, to, extraWhere, extraArgs, needJoin)
		if err != nil {
			return nil, err
		}
		return funnelResults(f, counts), nil
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
				JOIN sessions s ON s.id = e.session_id AND s.project_id = e.project_id
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
		q, args = stepFilterClause(q, args, step)

		if err := s.rdb.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
			return nil, fmt.Errorf("analyze step %d: %w", i, err)
		}
		stepCounts[i] = n
	}
	return funnelResults(f, stepCounts), nil
}

// funnelResults turns per-step counts into conversion and drop-off rates.
func funnelResults(f Funnel, stepCounts []int64) []FunnelStepResult {
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
	return results
}

// stepFilterClause appends a step's property filters to a step query.
func stepFilterClause(q string, args []any, step FunnelStep) (string, []any) {
	for _, filter := range step.Filters {
		if !validPropertyName.MatchString(filter.Property) {
			continue
		}
		q += fmt.Sprintf(" AND json_extract(e.properties, '$.%s') = ?", filter.Property)
		args = append(args, filter.Value)
	}
	return q, args
}

// SessionsAtStep returns distinct session IDs that completed step stepOrder
// (1-based) but did NOT complete step stepOrder+1. If stepOrder equals the
// total number of steps, it returns sessions that completed all steps (converters).
func (s *ReadStore) SessionsAtStep(ctx context.Context, f Funnel, stepOrder int, from, to time.Time, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}
	if stepOrder < 1 || stepOrder > len(f.Steps) {
		return nil, fmt.Errorf("step_order %d out of range [1, %d]", stepOrder, len(f.Steps))
	}
	// Find the step definition (steps are stored 1-based).
	var targetStep FunnelStep
	for _, st := range f.Steps {
		if st.StepOrder == stepOrder {
			targetStep = st
			break
		}
	}

	// Sessions that reached this step.
	const reachedQ = `
		SELECT DISTINCT session_id
		FROM events
		WHERE project_id = ? AND name = ? AND occurred_at >= ? AND occurred_at <= ?`
	rows, err := s.rdb.QueryContext(ctx, reachedQ, f.ProjectID, targetStep.EventName, from, to)
	if err != nil {
		return nil, fmt.Errorf("SessionsAtStep reached: %w", err)
	}
	reached := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		reached[id] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// If this is the last step, return all converters.
	if stepOrder == len(f.Steps) {
		return sortedSessionIDs(reached, limit), nil
	}

	// Otherwise subtract sessions that also reached the next step.
	var nextStep FunnelStep
	for _, st := range f.Steps {
		if st.StepOrder == stepOrder+1 {
			nextStep = st
			break
		}
	}
	const nextQ = `
		SELECT DISTINCT session_id
		FROM events
		WHERE project_id = ? AND name = ? AND occurred_at >= ? AND occurred_at <= ?`
	nextRows, err := s.rdb.QueryContext(ctx, nextQ, f.ProjectID, nextStep.EventName, from, to)
	if err != nil {
		return nil, fmt.Errorf("SessionsAtStep next: %w", err)
	}
	defer nextRows.Close()
	for nextRows.Next() {
		var id string
		if err := nextRows.Scan(&id); err != nil {
			return nil, err
		}
		delete(reached, id)
	}
	if err := nextRows.Err(); err != nil {
		return nil, err
	}

	return sortedSessionIDs(reached, limit), nil
}

// sortedSessionIDs returns up to limit session ids in ascending order, so the
// same data gives the same list on every call.
func sortedSessionIDs(ids map[string]bool, limit int) []string {
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
