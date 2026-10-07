package repository

import "context"

// annotateUnmatchedSteps fills UnmatchedSteps on each funnel from the set of
// event names the project has actually emitted. One query for the whole list,
// not one per funnel.
//
// "Never emitted" means within the event retention window: a name the project
// sent once a year ago and whose events have since been purged reads as
// unmatched, which is the same thing the funnel itself experiences.
func (s *Store) annotateUnmatchedSteps(ctx context.Context, db querier, projectID string, funnels []Funnel) error {
	if len(funnels) == 0 {
		return nil
	}
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT name FROM events WHERE project_id = ?`, projectID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	seen := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		seen[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// A project that has sent nothing at all would have every step of every
	// funnel flagged, which says nothing useful — there is no evidence either
	// way yet, and the dashboard already shows the project as having no data.
	// Flag only once there is something to compare against.
	if len(seen) == 0 {
		return nil
	}

	for i := range funnels {
		var unmatched []string
		reported := make(map[string]struct{}, len(funnels[i].Steps))
		for _, step := range funnels[i].Steps {
			if _, ok := seen[step.EventName]; ok {
				continue
			}
			// A name repeated across steps is reported once.
			if _, dup := reported[step.EventName]; dup {
				continue
			}
			reported[step.EventName] = struct{}{}
			unmatched = append(unmatched, step.EventName)
		}
		funnels[i].UnmatchedSteps = unmatched
	}
	return nil
}
