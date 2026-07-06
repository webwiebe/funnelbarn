package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CanonicalFunnel is an instance-level funnel defined over canonical event keys
// and evaluated across multiple projects.
type CanonicalFunnel struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Scope       string                `json:"scope"`       // "session" (default) or "page_view"
	ProjectIDs  []string              `json:"project_ids"` // default scope; empty = all projects
	Segment     string                `json:"segment"`     // default preset segment; "" = none
	Steps       []CanonicalFunnelStep `json:"steps"`
	CreatedAt   time.Time             `json:"created_at"`
}

// CanonicalFunnelStep is one step, referencing a canonical event key.
type CanonicalFunnelStep struct {
	StepOrder    int    `json:"step_order"`
	CanonicalKey string `json:"canonical_key"`
	Label        string `json:"label"`
}

// ProjectFunnelBreakdown holds one project's contribution to a canonical funnel.
type ProjectFunnelBreakdown struct {
	ProjectID   string             `json:"project_id"`
	ProjectName string             `json:"project_name"`
	Steps       []FunnelStepResult `json:"steps"`
}

// ExcludedProject records a project dropped from a canonical funnel and why.
type ExcludedProject struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	Reason      string `json:"reason"`
}

// CanonicalFunnelResult is the aggregate analysis plus per-project detail.
type CanonicalFunnelResult struct {
	Steps            []FunnelStepResult       `json:"steps"` // aggregated across included projects
	ByProject        []ProjectFunnelBreakdown `json:"by_project"`
	ExcludedProjects []ExcludedProject        `json:"excluded_projects"`
}

// CreateCanonicalFunnel inserts a canonical funnel with its steps.
func (s *Store) CreateCanonicalFunnel(ctx context.Context, f CanonicalFunnel) (CanonicalFunnel, error) {
	var err error
	f.ID, err = generateUUID()
	if err != nil {
		return CanonicalFunnel{}, fmt.Errorf("generate uuid: %w", err)
	}
	if f.Scope == "" {
		f.Scope = "session"
	}
	projectIDsJSON, _ := json.Marshal(f.ProjectIDs)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CanonicalFunnel{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	const qf = `INSERT INTO canonical_funnels (id, name, description, scope, project_ids, segment) VALUES (?, ?, ?, ?, ?, ?)`
	if _, err := tx.ExecContext(ctx, qf, f.ID, f.Name, nullStr(f.Description), f.Scope, string(projectIDsJSON), nullStr(f.Segment)); err != nil {
		return CanonicalFunnel{}, fmt.Errorf("insert canonical funnel: %w", err)
	}
	if err := insertCanonicalSteps(ctx, tx, f.ID, f.Steps); err != nil {
		return CanonicalFunnel{}, err
	}
	if err := tx.Commit(); err != nil {
		return CanonicalFunnel{}, err
	}
	return s.CanonicalFunnelByID(ctx, f.ID)
}

// insertCanonicalSteps writes ordered steps for a canonical funnel within a tx.
func insertCanonicalSteps(ctx context.Context, tx *sql.Tx, funnelID string, steps []CanonicalFunnelStep) error {
	const qs = `INSERT INTO canonical_funnel_steps (id, funnel_id, step_order, canonical_key) VALUES (?, ?, ?, ?)`
	for i := range steps {
		stepID, err := generateUUID()
		if err != nil {
			return fmt.Errorf("generate uuid: %w", err)
		}
		order := i + 1
		if _, err := tx.ExecContext(ctx, qs, stepID, funnelID, order, steps[i].CanonicalKey); err != nil {
			if isForeignKeyViolation(err) {
				return fmt.Errorf("insert canonical step %d: unknown canonical key %q", order, steps[i].CanonicalKey)
			}
			return fmt.Errorf("insert canonical step %d: %w", order, err)
		}
	}
	return nil
}

// CanonicalFunnelByID fetches a canonical funnel with its steps (step labels are
// resolved from the canonical_events catalog).
func (s *Store) CanonicalFunnelByID(ctx context.Context, id string) (CanonicalFunnel, error) {
	const qf = `SELECT id, name, COALESCE(description,''), COALESCE(scope,'session'), COALESCE(project_ids,'[]'), COALESCE(segment,''), created_at FROM canonical_funnels WHERE id = ?`
	var f CanonicalFunnel
	var projectIDsJSON string
	if err := s.db.QueryRowContext(ctx, qf, id).Scan(&f.ID, &f.Name, &f.Description, &f.Scope, &projectIDsJSON, &f.Segment, &f.CreatedAt); err != nil {
		return CanonicalFunnel{}, err
	}
	_ = json.Unmarshal([]byte(projectIDsJSON), &f.ProjectIDs)
	steps, err := s.canonicalFunnelSteps(ctx, id)
	if err != nil {
		return CanonicalFunnel{}, err
	}
	f.Steps = steps
	return f, nil
}

// ListCanonicalFunnels returns all canonical funnels with their steps.
func (s *Store) ListCanonicalFunnels(ctx context.Context) ([]CanonicalFunnel, error) {
	const q = `SELECT id, name, COALESCE(description,''), COALESCE(scope,'session'), COALESCE(project_ids,'[]'), COALESCE(segment,''), created_at FROM canonical_funnels ORDER BY created_at`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var funnels []CanonicalFunnel
	for rows.Next() {
		var f CanonicalFunnel
		var projectIDsJSON string
		if err := rows.Scan(&f.ID, &f.Name, &f.Description, &f.Scope, &projectIDsJSON, &f.Segment, &f.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(projectIDsJSON), &f.ProjectIDs)
		funnels = append(funnels, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range funnels {
		steps, err := s.canonicalFunnelSteps(ctx, funnels[i].ID)
		if err != nil {
			return nil, err
		}
		funnels[i].Steps = steps
	}
	return funnels, nil
}

// UpdateCanonicalFunnel replaces a canonical funnel's fields and steps.
func (s *Store) UpdateCanonicalFunnel(ctx context.Context, f CanonicalFunnel) (CanonicalFunnel, error) {
	if f.Scope == "" {
		f.Scope = "session"
	}
	projectIDsJSON, _ := json.Marshal(f.ProjectIDs)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CanonicalFunnel{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx, `UPDATE canonical_funnels SET name=?, description=?, scope=?, project_ids=?, segment=? WHERE id=?`,
		f.Name, nullStr(f.Description), f.Scope, string(projectIDsJSON), nullStr(f.Segment), f.ID)
	if err != nil {
		return CanonicalFunnel{}, fmt.Errorf("update canonical funnel: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return CanonicalFunnel{}, sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM canonical_funnel_steps WHERE funnel_id=?`, f.ID); err != nil {
		return CanonicalFunnel{}, fmt.Errorf("delete canonical steps: %w", err)
	}
	if err := insertCanonicalSteps(ctx, tx, f.ID, f.Steps); err != nil {
		return CanonicalFunnel{}, err
	}
	if err := tx.Commit(); err != nil {
		return CanonicalFunnel{}, err
	}
	return s.CanonicalFunnelByID(ctx, f.ID)
}

// DeleteCanonicalFunnel removes a canonical funnel and its steps (cascade).
func (s *Store) DeleteCanonicalFunnel(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM canonical_funnels WHERE id = ?`, id)
	return err
}

// canonicalFunnelSteps returns steps ordered by step_order, resolving each
// canonical key's label from the catalog.
func (s *Store) canonicalFunnelSteps(ctx context.Context, funnelID string) ([]CanonicalFunnelStep, error) {
	const q = `
		SELECT st.step_order, st.canonical_key, COALESCE(ce.label, st.canonical_key)
		FROM canonical_funnel_steps st
		LEFT JOIN canonical_events ce ON ce.key = st.canonical_key
		WHERE st.funnel_id = ?
		ORDER BY st.step_order`
	rows, err := s.db.QueryContext(ctx, q, funnelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var steps []CanonicalFunnelStep
	for rows.Next() {
		var st CanonicalFunnelStep
		if err := rows.Scan(&st.StepOrder, &st.CanonicalKey, &st.Label); err != nil {
			return nil, err
		}
		steps = append(steps, st)
	}
	return steps, rows.Err()
}

// nameInClause builds an `e.name IN (?, ?, …)` fragment plus its bind args.
func nameInClause(names []string) (clause string, args []any) {
	ph := make([]string, len(names))
	for i, n := range names {
		ph[i] = "?"
		args = append(args, n)
	}
	return "e.name IN (" + strings.Join(ph, ",") + ")", args
}

// AnalyzeCanonicalFunnel computes drop-off for a canonical funnel aggregated
// across projects. Because a session (and page_view) belongs to exactly one
// project, the projects partition the population disjointly, so the global
// distinct count per step is the sum of the per-project distinct counts.
//
// projectIDs selects the projects to include; empty means all projects. A
// project that lacks a mapping for ANY step is excluded from the whole funnel
// and reported in ExcludedProjects. Segmentation reuses the same seg/rules
// machinery as AnalyzeFunnel.
func (s *Store) AnalyzeCanonicalFunnel(ctx context.Context, f CanonicalFunnel, projectIDs []string, from, to time.Time, seg *SegmentFilter, rules ...SegmentRule) (CanonicalFunnelResult, error) {
	var result CanonicalFunnelResult
	if len(f.Steps) == 0 {
		return result, nil
	}

	// Resolve project set + names.
	allProjects, err := s.ListProjects(ctx)
	if err != nil {
		return result, err
	}
	nameByID := make(map[string]string, len(allProjects))
	for _, p := range allProjects {
		nameByID[p.ID] = p.Name
	}
	var targetIDs []string
	if len(projectIDs) > 0 {
		targetIDs = projectIDs
	} else {
		for _, p := range allProjects {
			targetIDs = append(targetIDs, p.ID)
		}
	}

	mappings, err := s.MappingsByProject(ctx)
	if err != nil {
		return result, err
	}

	// Build the shared segment WHERE fragment once (identical to AnalyzeFunnel).
	presetClause, presetArg, needJoin := segmentParam(seg)
	ruleClause, ruleArgs, rulesNeedJoin := buildSegmentRuleClause(rules)
	needJoin = needJoin || rulesNeedJoin

	scope := f.Scope
	if scope == "" {
		scope = "session"
	}
	pageViewScope := scope == "page_view"
	distinctCol := "e.session_id"
	if pageViewScope {
		distinctCol = "e.page_view_id"
	}

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

	agg := make([]int64, len(f.Steps))
	for _, pid := range targetIDs {
		canonMap := mappings[pid]

		// Exclusion rule: a project missing ANY step's mapping is dropped from
		// the entire funnel and reported.
		missingKey := ""
		for _, step := range f.Steps {
			if len(canonMap[step.CanonicalKey]) == 0 {
				missingKey = step.CanonicalKey
				break
			}
		}
		if missingKey != "" {
			result.ExcludedProjects = append(result.ExcludedProjects, ExcludedProject{
				ProjectID:   pid,
				ProjectName: nameByID[pid],
				Reason:      fmt.Sprintf("no event mapped to %q", missingKey),
			})
			continue
		}

		stepCounts := make([]int64, len(f.Steps))
		for i, step := range f.Steps {
			names := canonMap[step.CanonicalKey]
			inClause, inArgs := nameInClause(names)

			var q string
			if needJoin {
				q = fmt.Sprintf(`
					SELECT COUNT(DISTINCT %s)
					FROM events e
					JOIN sessions s ON s.id = e.session_id
					WHERE e.project_id = ? AND %s AND e.occurred_at >= ? AND e.occurred_at <= ?%s`,
					distinctCol, inClause, extraWhere)
			} else {
				q = fmt.Sprintf(`
					SELECT COUNT(DISTINCT %s)
					FROM events e
					WHERE e.project_id = ? AND %s AND e.occurred_at >= ? AND e.occurred_at <= ?%s`,
					distinctCol, inClause, extraWhere)
			}
			// Arg order mirrors AnalyzeFunnel: project_id, …names…, from, to, …segmentArgs…
			args := []any{pid}
			args = append(args, inArgs...)
			args = append(args, from, to)
			args = append(args, extraArgs...)

			var n int64
			if err := s.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
				return result, fmt.Errorf("analyze canonical step %d (project %s): %w", i, pid, err)
			}
			stepCounts[i] = n
			agg[i] += n
		}
		result.ByProject = append(result.ByProject, ProjectFunnelBreakdown{
			ProjectID:   pid,
			ProjectName: nameByID[pid],
			Steps:       buildStepResults(f.Steps, stepCounts),
		})
	}

	result.Steps = buildStepResults(f.Steps, agg)
	return result, nil
}

// buildStepResults turns raw per-step distinct counts into conversion/drop-off
// results relative to the entry (first) step. Mirrors AnalyzeFunnel's math.
func buildStepResults(steps []CanonicalFunnelStep, counts []int64) []FunnelStepResult {
	results := make([]FunnelStepResult, len(steps))
	var entry int64
	if len(counts) > 0 {
		entry = counts[0]
	}
	for i, step := range steps {
		r := FunnelStepResult{StepOrder: step.StepOrder, EventName: step.CanonicalKey, Count: counts[i]}
		if entry > 0 {
			r.Conversion = float64(counts[i]) / float64(entry)
		}
		if i > 0 && counts[i-1] > 0 {
			r.DropOff = 1.0 - float64(counts[i])/float64(counts[i-1])
		}
		results[i] = r
	}
	return results
}
