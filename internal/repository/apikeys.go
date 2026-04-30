package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository/sqlcgen"
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
	row, err := s.q.LookupAPIKeyBySHA256(ctx, keySHA256)
	if err != nil {
		if isNoRows(err) {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	return row.ProjectID, row.Scope, true, nil
}

// TouchAPIKey updates last_used_at for a key by its SHA256 hash.
func (s *Store) TouchAPIKey(ctx context.Context, keySHA256 string) error {
	return s.q.TouchAPIKey(ctx, keySHA256)
}

// EnsureSetupAPIKey upserts an ingest-scoped API key for a project using the provided SHA-256 hash.
// The key name is "setup" and the scope is "ingest". If a key with the same hash already exists,
// nothing changes (idempotent).
func (s *Store) EnsureSetupAPIKey(ctx context.Context, projectID, keySHA256 string) error {
	return s.q.EnsureSetupAPIKey(ctx, sqlcgen.EnsureSetupAPIKeyParams{
		ID:        newUUID(),
		ProjectID: projectID,
		KeyHash:   keySHA256,
	})
}

// CreateAPIKey inserts a new API key.
func (s *Store) CreateAPIKey(ctx context.Context, name, projectID, keySHA256, scope string) (APIKey, error) {
	id := newUUID()
	if err := s.q.InsertAPIKey(ctx, sqlcgen.InsertAPIKeyParams{
		ID:        id,
		ProjectID: projectID,
		Name:      name,
		KeyHash:   keySHA256,
		Scope:     scope,
	}); err != nil {
		return APIKey{}, fmt.Errorf("create api key: %w", err)
	}
	k, err := s.q.GetAPIKeyByID(ctx, id)
	if err != nil {
		return APIKey{}, err
	}
	return apiKeyFromGen(k), nil
}

// ListAPIKeys returns all API keys for a project.
func (s *Store) ListAPIKeys(ctx context.Context, projectID string) ([]APIKey, error) {
	rows, err := s.q.ListAPIKeysByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	keys := make([]APIKey, 0, len(rows))
	for _, k := range rows {
		keys = append(keys, apiKeyFromGen(k))
	}
	return keys, nil
}

// ListAllAPIKeys returns all API keys across all projects, ordered by creation time.
func (s *Store) ListAllAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.q.ListAllAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]APIKey, 0, len(rows))
	for _, k := range rows {
		keys = append(keys, apiKeyFromGen(k))
	}
	return keys, nil
}

// DeleteAPIKey removes an API key by its ID.
func (s *Store) DeleteAPIKey(ctx context.Context, id string) error {
	return s.q.DeleteAPIKey(ctx, id)
}

// isNoRows returns true if the error is sql.ErrNoRows.
func isNoRows(err error) bool {
	return err == sql.ErrNoRows
}

func apiKeyFromGen(k sqlcgen.ApiKey) APIKey {
	return APIKey{
		ID:        k.ID,
		ProjectID: k.ProjectID,
		Name:      k.Name,
		KeyHash:   k.KeyHash,
		Scope:     k.Scope,
		CreatedAt: k.CreatedAt,
	}
}
