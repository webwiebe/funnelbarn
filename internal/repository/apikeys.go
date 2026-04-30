package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// APIKey represents a stored API key.
type APIKey struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	KeyHash   string    `json:"-"`
	Scope     string    `json:"scope"`
	CreatedAt time.Time `json:"created_at"`
}

// ValidAPIKeySHA256 looks up an API key by its SHA256 hex digest.
// Returns (projectID, scope, true, nil) on match.
func (s *Store) ValidAPIKeySHA256(ctx context.Context, keySHA256 string) (projectID string, scope string, found bool, err error) {
	const q = `
		SELECT project_id, scope FROM api_keys
		WHERE key_hash = ?
		LIMIT 1`
	row := s.db.QueryRowContext(ctx, q, keySHA256)
	err = row.Scan(&projectID, &scope)
	if err != nil {
		if isNoRows(err) {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	return projectID, scope, true, nil
}

// TouchAPIKey updates last_used_at for a key by its SHA256 hash.
func (s *Store) TouchAPIKey(ctx context.Context, keySHA256 string) error {
	const q = `UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE key_hash = ?`
	_, err := s.db.ExecContext(ctx, q, keySHA256)
	return err
}

// EnsureSetupAPIKey upserts an ingest-scoped API key for a project using the provided SHA-256 hash.
// The key name is "setup" and the scope is "ingest". If a key with the same hash already exists,
// nothing changes (idempotent).
func (s *Store) EnsureSetupAPIKey(ctx context.Context, projectID, keySHA256 string) error {
	const q = `
		INSERT INTO api_keys (id, project_id, name, key_hash, scope)
		VALUES (?, ?, 'setup', ?, 'ingest')
		ON CONFLICT(key_hash) DO NOTHING`
	_, err := s.db.ExecContext(ctx, q, newUUID(), projectID, keySHA256)
	return err
}

// CreateAPIKey inserts a new API key.
func (s *Store) CreateAPIKey(ctx context.Context, name, projectID, keySHA256, scope string) (APIKey, error) {
	id := newUUID()
	const q = `INSERT INTO api_keys (id, project_id, name, key_hash, scope) VALUES (?, ?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q, id, projectID, name, keySHA256, scope); err != nil {
		return APIKey{}, fmt.Errorf("create api key: %w", err)
	}
	var k APIKey
	const sel = `SELECT id, project_id, name, key_hash, scope, created_at FROM api_keys WHERE id = ?`
	err := s.db.QueryRowContext(ctx, sel, id).Scan(&k.ID, &k.ProjectID, &k.Name, &k.KeyHash, &k.Scope, &k.CreatedAt)
	return k, err
}

// ListAPIKeys returns all API keys for a project.
func (s *Store) ListAPIKeys(ctx context.Context, projectID string) ([]APIKey, error) {
	const q = `SELECT id, project_id, name, key_hash, scope, created_at FROM api_keys WHERE project_id = ? ORDER BY created_at`
	rows, err := s.db.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.Name, &k.KeyHash, &k.Scope, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// ListAllAPIKeys returns all API keys across all projects, ordered by creation time.
func (s *Store) ListAllAPIKeys(ctx context.Context) ([]APIKey, error) {
	const q = `SELECT id, project_id, name, key_hash, scope, created_at FROM api_keys ORDER BY created_at`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.Name, &k.KeyHash, &k.Scope, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// DeleteAPIKey removes an API key by its ID.
func (s *Store) DeleteAPIKey(ctx context.Context, id string) error {
	const q = `DELETE FROM api_keys WHERE id = ?`
	_, err := s.db.ExecContext(ctx, q, id)
	return err
}

// isNoRows returns true if the error is sql.ErrNoRows.
func isNoRows(err error) bool {
	return err == sql.ErrNoRows
}
