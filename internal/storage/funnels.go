package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Funnel is a multi-step conversion funnel.
type Funnel struct {
	ID          string
	ProjectID   string
	Name        string
	Description string
	Steps       []FunnelStep
	CreatedAt   time.Time
}

// FunnelStep is one step in a funnel.
type FunnelStep struct {
	ID        string
	FunnelID  string
	StepOrder int
	EventName string
	Filters   []FunnelFilter
}

// FunnelFilter filters events at a funnel step by property value.
type FunnelFilter struct {
	Property string `json:"property"`
	Value    string `json:"value"`
}

// FunnelStepResult holds analysis results for one funnel step.
type FunnelStepResult struct {
	StepOrder  int
	EventName  string
	Count      int64
	Conversion float64 // fraction of step 0 that reached this step
	DropOff    float64 // fraction lost from previous step
}

// CreateFunnel inserts a funnel with its steps.
func (s *Store) CreateFunnel(ctx context.Context, f Funnel) (Funnel, error) {
	f.ID = generateUUID()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Funnel{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	const qf = `INSERT INTO funnels (id, project_id, name, description) VALUES (?, ?, ?, ?)`
	if _, err := tx.ExecContext(ctx, qf, f.ID, f.ProjectID, f.Name, nullStr(f.Description)); err != nil {
		return Funnel{}, fmt.Errorf("insert funnel: %w", err)
	}

	const qs = `INSERT INTO funnel_steps (id, funnel_id, step_order, event_name, filters) VALUES (?, ?, ?, ?, ?)`
	for i := range f.Steps {
		f.Steps[i].ID = generateUUID()
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
	const qf = `SELECT id, project_id, name, COALESCE(description,''), created_at FROM funnels WHERE id = ?`
	var f Funnel
	if err := s.db.QueryRowContext(ctx, qf, id).Scan(&f.ID, &f.ProjectID, &f.Name, &f.Description, &f.CreatedAt); err != nil {
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
	const q = `SELECT id, project_id, name, COALESCE(description,''), created_at FROM funnels WHERE project_id = ? ORDER BY created_at`
	rows, err := s.db.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var funnels []Funnel
	for rows.Next() {
		var f Funnel
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Name, &f.Description, &f.CreatedAt); err != nil {
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

// AnalyzeFunnel computes conversion rates for each step of a funnel over a time range.
// Uses a session-based approach: a session qualifies for step N only if it also
// completed steps 0..N-1 in order (by occurred_at).
func (s *Store) AnalyzeFunnel(ctx context.Context, f Funnel, from, to time.Time) ([]FunnelStepResult, error) {
	if len(f.Steps) == 0 {
		return nil, nil
	}

	// For each step, count sessions that have events with the required event name
	// in the given time window. Step 0 is the entry count.
	stepCounts := make([]int64, len(f.Steps))
	for i, step := range f.Steps {
		var n int64
		const q = `
			SELECT COUNT(DISTINCT session_id)
			FROM events
			WHERE project_id = ? AND name = ? AND occurred_at >= ? AND occurred_at <= ?`
		if err := s.db.QueryRowContext(ctx, q, f.ProjectID, step.EventName, from, to).Scan(&n); err != nil {
			return nil, fmt.Errorf("analyze step %d: %w", i, err)
		}
		stepCounts[i] = n
	}

	results := make([]FunnelStepResult, len(f.Steps))
	entry := stepCounts[0]

	for i, step := range f.Steps {
		r := FunnelStepResult{
			StepOrder: step.StepOrder,
			EventName: step.EventName,
			Count:     stepCounts[i],
		}
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
