package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/sqlcgen"
)

// ValidAPIKeySHA256 looks up an API key by its SHA256 hex digest.
// Returns (projectID, scope, true, nil) on match.
func (s *ReadStore) ValidAPIKeySHA256(ctx context.Context, keySHA256 string) (projectID string, scope string, found bool, err error) {
	row, err := s.rq.LookupAPIKeyBySHA256(ctx, keySHA256)
	if err != nil {
		if isNoRows(err) {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	return row.ProjectID, row.Scope, true, nil
}

// APIKeyEnvironmentSHA256 returns the environment a key is mapped to, or ""
// when the key is unknown or chooses none.
func (s *ReadStore) APIKeyEnvironmentSHA256(ctx context.Context, keySHA256 string) (string, error) {
	env, err := s.rq.LookupAPIKeyEnvironmentBySHA256(ctx, keySHA256)
	if err != nil {
		if isNoRows(err) {
			return "", nil
		}
		return "", err
	}
	return env, nil
}

// SetAPIKeyEnvironment maps a key to an environment ("" clears the mapping).
// The caller passes a canonical value. An unknown id is domain.ErrNotFound.
func (s *Store) SetAPIKeyEnvironment(ctx context.Context, id, env string) error {
	n, err := s.q.SetAPIKeyEnvironment(ctx, sqlcgen.SetAPIKeyEnvironmentParams{Environment: env, ID: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("api key %q: %w", id, domain.ErrNotFound)
	}
	return nil
}

// TouchAPIKey updates last_used_at for a key by its SHA256 hash.
func (s *Store) TouchAPIKey(ctx context.Context, keySHA256 string) error {
	return s.q.TouchAPIKey(ctx, keySHA256)
}

// EnsureSetupAPIKey upserts an ingest-scoped API key for a project using the provided SHA-256 hash.
// The key name is "setup" and the scope is "ingest". If a key with the same hash already exists,
// nothing changes (idempotent).
func (s *Store) EnsureSetupAPIKey(ctx context.Context, projectID, keySHA256 string) error {
	id, err := generateUUID()
	if err != nil {
		return fmt.Errorf("ensure setup api key: %w", err)
	}
	return s.q.EnsureSetupAPIKey(ctx, sqlcgen.EnsureSetupAPIKeyParams{
		ID:        id,
		ProjectID: projectID,
		KeyHash:   keySHA256,
	})
}

// CreateAPIKey inserts a new API key.
func (s *Store) CreateAPIKey(ctx context.Context, name, projectID, keySHA256, scope string) (domain.APIKey, error) {
	id, err := generateUUID()
	if err != nil {
		return domain.APIKey{}, fmt.Errorf("create api key: %w", err)
	}
	if err := s.q.InsertAPIKey(ctx, sqlcgen.InsertAPIKeyParams{
		ID:        id,
		ProjectID: projectID,
		Name:      name,
		KeyHash:   keySHA256,
		Scope:     scope,
	}); err != nil {
		return domain.APIKey{}, fmt.Errorf("create api key: %w", err)
	}
	k, err := s.q.GetAPIKeyByID(ctx, id)
	if err != nil {
		return domain.APIKey{}, err
	}
	return apiKeyFromGen(k), nil
}

// ListAPIKeys returns all API keys for a project.
func (s *ReadStore) ListAPIKeys(ctx context.Context, projectID string) ([]domain.APIKey, error) {
	rows, err := s.rq.ListAPIKeysByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	keys := make([]domain.APIKey, 0, len(rows))
	for _, k := range rows {
		keys = append(keys, apiKeyFromGen(k))
	}
	return keys, nil
}

// ListAllAPIKeys returns all API keys across all projects, ordered by creation time.
func (s *ReadStore) ListAllAPIKeys(ctx context.Context) ([]domain.APIKey, error) {
	rows, err := s.rq.ListAllAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]domain.APIKey, 0, len(rows))
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

func apiKeyFromGen(k sqlcgen.ApiKey) domain.APIKey {
	key := domain.APIKey{
		ID:          k.ID,
		ProjectID:   k.ProjectID,
		Name:        k.Name,
		KeyHash:     k.KeyHash,
		Scope:       k.Scope,
		CreatedAt:   k.CreatedAt,
		Environment: k.Environment,
	}
	if k.LastUsedAt.Valid {
		t := k.LastUsedAt.Time
		key.LastUsedAt = &t
	}
	return key
}
