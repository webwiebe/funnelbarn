package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// CreateCanonicalFunnel inserts a canonical funnel with its steps.
func (s *Store) CreateCanonicalFunnel(ctx context.Context, f domain.CanonicalFunnel) (domain.CanonicalFunnel, error) {
	var err error
	f.ID, err = generateUUID()
	if err != nil {
		return domain.CanonicalFunnel{}, fmt.Errorf("generate uuid: %w", err)
	}
	if f.Scope == "" {
		f.Scope = "session"
	}
	projectIDsJSON, _ := json.Marshal(f.ProjectIDs)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.CanonicalFunnel{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	const qf = `INSERT INTO canonical_funnels (id, name, description, scope, project_ids, segment) VALUES (?, ?, ?, ?, ?, ?)`
	if _, err := tx.ExecContext(ctx, qf, f.ID, f.Name, nullStr(f.Description), f.Scope, string(projectIDsJSON), nullStr(f.Segment)); err != nil {
		return domain.CanonicalFunnel{}, fmt.Errorf("insert canonical funnel: %w", err)
	}
	if err := insertCanonicalSteps(ctx, tx, f.ID, f.Steps); err != nil {
		return domain.CanonicalFunnel{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.CanonicalFunnel{}, err
	}
	return s.canonicalFunnelByID(ctx, s.db, f.ID)
}

// insertCanonicalSteps writes ordered steps for a canonical funnel within a tx.
func insertCanonicalSteps(ctx context.Context, tx *sql.Tx, funnelID string, steps []domain.CanonicalFunnelStep) error {
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
func (s *ReadStore) CanonicalFunnelByID(ctx context.Context, id string) (domain.CanonicalFunnel, error) {
	return s.canonicalFunnelByID(ctx, s.rdb, id)
}

// canonicalFunnelByID reads through db, so write paths can read back on the write pool.
func (s *ReadStore) canonicalFunnelByID(ctx context.Context, db querier, id string) (domain.CanonicalFunnel, error) {
	const qf = `SELECT id, name, COALESCE(description,''), COALESCE(scope,'session'), COALESCE(project_ids,'[]'), COALESCE(segment,''), created_at FROM canonical_funnels WHERE id = ?`
	var f domain.CanonicalFunnel
	var projectIDsJSON string
	if err := db.QueryRowContext(ctx, qf, id).Scan(&f.ID, &f.Name, &f.Description, &f.Scope, &projectIDsJSON, &f.Segment, &f.CreatedAt); err != nil {
		return domain.CanonicalFunnel{}, err
	}
	if err := json.Unmarshal([]byte(projectIDsJSON), &f.ProjectIDs); err != nil {
		slog.WarnContext(ctx, "canonical funnel: malformed project_ids JSON", "funnel_id", f.ID, "raw_len", len(projectIDsJSON), "error", err)
	}
	steps, err := s.canonicalFunnelSteps(ctx, db, id)
	if err != nil {
		return domain.CanonicalFunnel{}, err
	}
	f.Steps = steps
	return f, nil
}

// ListCanonicalFunnels returns all canonical funnels with their steps.
func (s *ReadStore) ListCanonicalFunnels(ctx context.Context) ([]domain.CanonicalFunnel, error) {
	const q = `SELECT id, name, COALESCE(description,''), COALESCE(scope,'session'), COALESCE(project_ids,'[]'), COALESCE(segment,''), created_at FROM canonical_funnels ORDER BY created_at`
	rows, err := s.rdb.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var funnels []domain.CanonicalFunnel
	for rows.Next() {
		var f domain.CanonicalFunnel
		var projectIDsJSON string
		if err := rows.Scan(&f.ID, &f.Name, &f.Description, &f.Scope, &projectIDsJSON, &f.Segment, &f.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(projectIDsJSON), &f.ProjectIDs); err != nil {
			slog.WarnContext(ctx, "canonical funnel: malformed project_ids JSON", "funnel_id", f.ID, "raw_len", len(projectIDsJSON), "error", err)
		}
		funnels = append(funnels, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range funnels {
		steps, err := s.canonicalFunnelSteps(ctx, s.rdb, funnels[i].ID)
		if err != nil {
			return nil, err
		}
		funnels[i].Steps = steps
	}
	return funnels, nil
}

// UpdateCanonicalFunnel replaces a canonical funnel's fields and steps.
func (s *Store) UpdateCanonicalFunnel(ctx context.Context, f domain.CanonicalFunnel) (domain.CanonicalFunnel, error) {
	if f.Scope == "" {
		f.Scope = "session"
	}
	projectIDsJSON, _ := json.Marshal(f.ProjectIDs)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.CanonicalFunnel{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx, `UPDATE canonical_funnels SET name=?, description=?, scope=?, project_ids=?, segment=? WHERE id=?`,
		f.Name, nullStr(f.Description), f.Scope, string(projectIDsJSON), nullStr(f.Segment), f.ID)
	if err != nil {
		return domain.CanonicalFunnel{}, fmt.Errorf("update canonical funnel: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.CanonicalFunnel{}, sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM canonical_funnel_steps WHERE funnel_id=?`, f.ID); err != nil {
		return domain.CanonicalFunnel{}, fmt.Errorf("delete canonical steps: %w", err)
	}
	if err := insertCanonicalSteps(ctx, tx, f.ID, f.Steps); err != nil {
		return domain.CanonicalFunnel{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.CanonicalFunnel{}, err
	}
	return s.canonicalFunnelByID(ctx, s.db, f.ID)
}

// DeleteCanonicalFunnel removes a canonical funnel and its steps (cascade).
func (s *Store) DeleteCanonicalFunnel(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM canonical_funnels WHERE id = ?`, id)
	return err
}

// canonicalFunnelSteps returns steps ordered by step_order, resolving each
// canonical key's label from the catalog.
func (s *ReadStore) canonicalFunnelSteps(ctx context.Context, db querier, funnelID string) ([]domain.CanonicalFunnelStep, error) {
	const q = `
		SELECT st.step_order, st.canonical_key, COALESCE(ce.label, st.canonical_key)
		FROM canonical_funnel_steps st
		LEFT JOIN canonical_events ce ON ce.key = st.canonical_key
		WHERE st.funnel_id = ?
		ORDER BY st.step_order`
	rows, err := db.QueryContext(ctx, q, funnelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var steps []domain.CanonicalFunnelStep
	for rows.Next() {
		var st domain.CanonicalFunnelStep
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
func (s *ReadStore) AnalyzeCanonicalFunnel(ctx context.Context, f domain.CanonicalFunnel, projectIDs []string, from, to time.Time, seg *domain.SegmentFilter, rules ...domain.SegmentRule) (domain.CanonicalFunnelResult, error) {
	var result domain.CanonicalFunnelResult
	if len(f.Steps) == 0 {
		return result, nil
	}

	// Resolve project set + names.
	targetIDs, nameByID, err := s.canonicalProjectSet(ctx, projectIDs)
	if err != nil {
		return result, err
	}
	mappings, err := s.MappingsByProject(ctx)
	if err != nil {
		return result, err
	}

	// Build the shared segment WHERE fragment once (identical to AnalyzeFunnel).
	w := canonicalWhere(f.Scope, seg, rules)

	agg := make([]int64, len(f.Steps))
	for _, pid := range targetIDs {
		canonMap := mappings[pid]

		// Exclusion rule: a project missing ANY step's mapping is dropped from
		// the entire funnel and reported.
		if missingKey := missingCanonicalMapping(f.Steps, canonMap); missingKey != "" {
			result.ExcludedProjects = append(result.ExcludedProjects, domain.ExcludedProject{
				ProjectID:   pid,
				ProjectName: nameByID[pid],
				Reason:      fmt.Sprintf("no event mapped to %q", missingKey),
			})
			continue
		}

		stepCounts, err := s.canonicalStepCounts(ctx, f.Steps, canonMap, pid, from, to, w)
		if err != nil {
			return result, err
		}
		for i, n := range stepCounts {
			agg[i] += n
		}
		result.ByProject = append(result.ByProject, domain.ProjectFunnelBreakdown{
			ProjectID:   pid,
			ProjectName: nameByID[pid],
			Steps:       buildStepResults(f.Steps, stepCounts),
		})
	}

	result.Steps = buildStepResults(f.Steps, agg)
	return result, nil
}

// canonicalProjectSet returns the projects to analyse (the requested ones, or
// all when none are named) and every project's name by id.
func (s *ReadStore) canonicalProjectSet(ctx context.Context, projectIDs []string) ([]string, map[string]string, error) {
	allProjects, err := s.ListProjects(ctx)
	if err != nil {
		return nil, nil, err
	}
	nameByID := make(map[string]string, len(allProjects))
	for _, p := range allProjects {
		nameByID[p.ID] = p.Name
	}
	if len(projectIDs) > 0 {
		return projectIDs, nameByID, nil
	}
	var targetIDs []string
	for _, p := range allProjects {
		targetIDs = append(targetIDs, p.ID)
	}
	return targetIDs, nameByID, nil
}

// canonicalFilter is the segment, rule and scope part shared by every step
// query of a canonical funnel.
type canonicalFilter struct {
	needJoin    bool   // the sessions table is needed for the segment or rules
	distinctCol string // the column counted per step
	extraWhere  string // " AND ..." fragment, empty when there is none
	extraArgs   []any
}

// canonicalWhere builds the shared filter. An empty scope means session.
func canonicalWhere(scope string, seg *domain.SegmentFilter, rules []domain.SegmentRule) canonicalFilter {
	presetClause, presetArg, needJoin := segmentParam(seg)
	ruleClause, ruleArgs, rulesNeedJoin := buildSegmentRuleClause(rules)

	pageViewScope := scope == "page_view"
	w := canonicalFilter{needJoin: needJoin || rulesNeedJoin, distinctCol: "e.session_id"}
	if pageViewScope {
		w.distinctCol = "e.page_view_id"
	}

	var clauses []string
	if presetClause != "" {
		clauses = append(clauses, presetClause)
		if presetArg != nil {
			w.extraArgs = append(w.extraArgs, presetArg)
		}
	}
	if ruleClause != "" {
		clauses = append(clauses, ruleClause)
		w.extraArgs = append(w.extraArgs, ruleArgs...)
	}
	if pageViewScope {
		clauses = append(clauses, "e.page_view_id IS NOT NULL")
	}
	if len(clauses) > 0 {
		w.extraWhere = " AND " + strings.Join(clauses, " AND ")
	}
	return w
}

// missingCanonicalMapping returns the first step's canonical key that the
// project has no event mapped to, or "" when every step is mapped.
func missingCanonicalMapping(steps []domain.CanonicalFunnelStep, canonMap map[string][]string) string {
	for _, step := range steps {
		if len(canonMap[step.CanonicalKey]) == 0 {
			return step.CanonicalKey
		}
	}
	return ""
}

// canonicalStepCounts counts the distinct sessions (or page views) of one
// project per step.
func (s *ReadStore) canonicalStepCounts(ctx context.Context, steps []domain.CanonicalFunnelStep, canonMap map[string][]string, pid string, from, to time.Time, w canonicalFilter) ([]int64, error) {
	counts := make([]int64, len(steps))
	for i, step := range steps {
		inClause, inArgs := nameInClause(canonMap[step.CanonicalKey])

		join := ""
		if w.needJoin {
			join = "\n\t\t\t\t\tJOIN sessions s ON s.id = e.session_id AND s.project_id = e.project_id"
		}
		q := fmt.Sprintf(`
					SELECT COUNT(DISTINCT %s)
					FROM events e%s
					WHERE e.project_id = ? AND %s AND e.occurred_at >= ? AND e.occurred_at <= ?%s`,
			w.distinctCol, join, inClause, w.extraWhere)
		// Arg order mirrors AnalyzeFunnel: project_id, …names…, from, to, …segmentArgs…
		args := []any{pid}
		args = append(args, inArgs...)
		args = append(args, from, to)
		args = append(args, w.extraArgs...)

		if err := s.rdb.QueryRowContext(ctx, q, args...).Scan(&counts[i]); err != nil {
			return nil, fmt.Errorf("analyze canonical step %d (project %s): %w", i, pid, err)
		}
	}
	return counts, nil
}

// buildStepResults turns raw per-step distinct counts into conversion/drop-off
// results relative to the entry (first) step. Mirrors AnalyzeFunnel's math.
func buildStepResults(steps []domain.CanonicalFunnelStep, counts []int64) []domain.FunnelStepResult {
	results := make([]domain.FunnelStepResult, len(steps))
	var entry int64
	if len(counts) > 0 {
		entry = counts[0]
	}
	for i, step := range steps {
		r := domain.FunnelStepResult{StepOrder: step.StepOrder, EventName: step.CanonicalKey, Count: counts[i]}
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
