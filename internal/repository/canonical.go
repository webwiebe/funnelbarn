package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// CanonicalEvent is an instance-level canonical event in the shared vocabulary.
type CanonicalEvent struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	SortOrder int    `json:"sort_order"`
}

// EventNameMapping maps one project's raw event name onto a canonical key.
type EventNameMapping struct {
	ProjectID    string `json:"project_id"`
	RawName      string `json:"raw_name"`
	CanonicalKey string `json:"canonical_key"`
}

// MappingSuggestion is an unmapped raw event name with a best-guess canonical key.
type MappingSuggestion struct {
	RawName      string `json:"raw_name"`
	SuggestedKey string `json:"suggested_key"` // "" when no confident guess
}

// ListCanonicalEvents returns the canonical event catalog ordered for display.
func (s *Store) ListCanonicalEvents(ctx context.Context) ([]CanonicalEvent, error) {
	const q = `SELECT key, label, sort_order FROM canonical_events ORDER BY sort_order, key`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CanonicalEvent
	for rows.Next() {
		var c CanonicalEvent
		if err := rows.Scan(&c.Key, &c.Label, &c.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateCanonicalEvent inserts a new canonical event. A duplicate key is a conflict.
func (s *Store) CreateCanonicalEvent(ctx context.Context, c CanonicalEvent) (CanonicalEvent, error) {
	const q = `INSERT INTO canonical_events (key, label, sort_order) VALUES (?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q, c.Key, c.Label, c.SortOrder); err != nil {
		if isUniqueViolation(err) {
			return CanonicalEvent{}, fmt.Errorf("canonical event %q: %w", c.Key, domain.ErrConflict)
		}
		return CanonicalEvent{}, err
	}
	return c, nil
}

// UpdateCanonicalEvent updates a canonical event's label and sort order.
func (s *Store) UpdateCanonicalEvent(ctx context.Context, c CanonicalEvent) (CanonicalEvent, error) {
	const q = `UPDATE canonical_events SET label = ?, sort_order = ? WHERE key = ?`
	res, err := s.db.ExecContext(ctx, q, c.Label, c.SortOrder, c.Key)
	if err != nil {
		return CanonicalEvent{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return CanonicalEvent{}, fmt.Errorf("canonical event %q: %w", c.Key, domain.ErrNotFound)
	}
	return c, nil
}

// DeleteCanonicalEvent removes a canonical event. A canonical event referenced
// by a saved funnel step cannot be deleted (conflict). The FK declarations are
// not enforced under the modernc SQLite driver, so both the reference check and
// the mapping cleanup are done explicitly here.
func (s *Store) DeleteCanonicalEvent(ctx context.Context, key string) error {
	var refs int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM canonical_funnel_steps WHERE canonical_key = ?`, key).Scan(&refs); err != nil {
		return err
	}
	if refs > 0 {
		return fmt.Errorf("canonical event %q is used by a funnel: %w", key, domain.ErrConflict)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM event_name_mappings WHERE canonical_key = ?`, key); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM canonical_events WHERE key = ?`, key)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("canonical event %q: %w", key, domain.ErrNotFound)
	}
	return nil
}

// CanonicalKeySet returns the set of catalog keys for existence validation.
func (s *Store) CanonicalKeySet(ctx context.Context) (map[string]bool, error) {
	events, err := s.ListCanonicalEvents(ctx)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(events))
	for _, e := range events {
		set[e.Key] = true
	}
	return set, nil
}

// ListMappings returns all raw→canonical mappings for a project.
func (s *Store) ListMappings(ctx context.Context, projectID string) ([]EventNameMapping, error) {
	const q = `SELECT project_id, raw_name, canonical_key FROM event_name_mappings WHERE project_id = ? ORDER BY raw_name`
	rows, err := s.db.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EventNameMapping
	for rows.Next() {
		var m EventNameMapping
		if err := rows.Scan(&m.ProjectID, &m.RawName, &m.CanonicalKey); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UpsertMapping creates or updates the canonical key for a project's raw name.
func (s *Store) UpsertMapping(ctx context.Context, projectID, rawName, canonicalKey string) error {
	const q = `
		INSERT INTO event_name_mappings (project_id, raw_name, canonical_key)
		VALUES (?, ?, ?)
		ON CONFLICT(project_id, raw_name) DO UPDATE SET canonical_key = excluded.canonical_key`
	if _, err := s.db.ExecContext(ctx, q, projectID, rawName, canonicalKey); err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("canonical key %q: %w", canonicalKey, domain.ErrNotFound)
		}
		return err
	}
	return nil
}

// DeleteMapping removes a raw→canonical mapping for a project.
func (s *Store) DeleteMapping(ctx context.Context, projectID, rawName string) error {
	const q = `DELETE FROM event_name_mappings WHERE project_id = ? AND raw_name = ?`
	_, err := s.db.ExecContext(ctx, q, projectID, rawName)
	return err
}

// MappingsByProject returns every mapping grouped as
// result[projectID][canonicalKey] = []rawName. It is the input to the aggregate
// funnel engine — one query, grouped in Go.
func (s *Store) MappingsByProject(ctx context.Context) (map[string]map[string][]string, error) {
	const q = `SELECT project_id, canonical_key, raw_name FROM event_name_mappings ORDER BY project_id, canonical_key`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]map[string][]string)
	for rows.Next() {
		var projectID, canonicalKey, rawName string
		if err := rows.Scan(&projectID, &canonicalKey, &rawName); err != nil {
			return nil, err
		}
		byCanon := out[projectID]
		if byCanon == nil {
			byCanon = make(map[string][]string)
			out[projectID] = byCanon
		}
		byCanon[canonicalKey] = append(byCanon[canonicalKey], rawName)
	}
	return out, rows.Err()
}

// MappingSuggestions returns unmapped raw event names for a project, each with a
// best-guess canonical key (empty when uncertain). Already-mapped raw names are
// excluded. Suggestions are advisory — they are persisted only when confirmed.
func (s *Store) MappingSuggestions(ctx context.Context, projectID string) ([]MappingSuggestion, error) {
	rawNames, err := s.DistinctEventNames(ctx, projectID)
	if err != nil {
		return nil, err
	}
	existing, err := s.ListMappings(ctx, projectID)
	if err != nil {
		return nil, err
	}
	mapped := make(map[string]bool, len(existing))
	for _, m := range existing {
		mapped[m.RawName] = true
	}
	catalog, err := s.ListCanonicalEvents(ctx)
	if err != nil {
		return nil, err
	}
	catalogKeys := make(map[string]bool, len(catalog))
	for _, c := range catalog {
		catalogKeys[c.Key] = true
	}

	var out []MappingSuggestion
	for _, raw := range rawNames {
		if mapped[raw] {
			continue
		}
		out = append(out, MappingSuggestion{
			RawName:      raw,
			SuggestedKey: guessCanonicalKey(raw, catalogKeys),
		})
	}
	return out, nil
}

// isUniqueViolation reports whether err is a SQLite UNIQUE/PRIMARY KEY conflict.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// isForeignKeyViolation reports whether err is a SQLite foreign-key failure.
func isForeignKeyViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
