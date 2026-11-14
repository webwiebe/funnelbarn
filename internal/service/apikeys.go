package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
)

// APIKeyService handles API key business logic.
type APIKeyService struct {
	store ports.APIKeyRepo
}

// NewAPIKeyService creates a new APIKeyService.
func NewAPIKeyService(store ports.APIKeyRepo) *APIKeyService {
	return &APIKeyService{store: store}
}

// randRead fills b from the system random source. Tests replace it to force a
// generation failure.
var randRead = rand.Read

// KeyGenerationError reports that the random source failed while minting a key.
// Callers match it with errors.As and map it to an internal error.
type KeyGenerationError struct{ Err error }

func (e *KeyGenerationError) Error() string { return e.Err.Error() }

func (e *KeyGenerationError) Unwrap() error { return e.Err }

// IssueAPIKey mints a key: 32 random bytes, hex encoded, stored as its SHA-256.
// It returns the stored key and the plaintext, which is shown to the caller
// once and never kept.
func (svc *APIKeyService) IssueAPIKey(ctx context.Context, name, projectID, scope string) (domain.APIKey, string, error) {
	var raw [32]byte
	if _, err := randRead(raw[:]); err != nil {
		return domain.APIKey{}, "", &KeyGenerationError{Err: err}
	}
	plaintext := hex.EncodeToString(raw[:])
	sum := sha256.Sum256([]byte(plaintext))
	key, err := svc.CreateAPIKey(ctx, name, projectID, hex.EncodeToString(sum[:]), scope)
	if err != nil {
		return domain.APIKey{}, "", err
	}
	return key, plaintext, nil
}

func (svc *APIKeyService) CreateAPIKey(ctx context.Context, name, projectID, keySHA256, scope string) (domain.APIKey, error) {
	if strings.TrimSpace(name) == "" {
		return domain.APIKey{}, &domain.ValidationError{Field: "name", Message: "required"}
	}
	if strings.TrimSpace(projectID) == "" {
		return domain.APIKey{}, &domain.ValidationError{Field: "project_id", Message: "required"}
	}
	if !validAPIKeyScope(scope) {
		return domain.APIKey{}, &domain.ValidationError{
			Field:   "scope",
			Message: `must be one of "full", "ingest", "analytics:read", "flags:read", "flags:write"`,
		}
	}
	return svc.store.CreateAPIKey(ctx, name, projectID, keySHA256, scope)
}

// validAPIKeyScope gates what a key may be issued with. Anything not listed is
// refused outright rather than stored and silently treated as no access.
func validAPIKeyScope(scope string) bool {
	switch scope {
	case domain.APIKeyScopeFull,
		domain.APIKeyScopeIngest,
		domain.APIKeyScopeAnalyticsRead,
		domain.APIKeyScopeFlagsRead,
		domain.APIKeyScopeFlagsWrite:
		return true
	}
	return false
}

func (svc *APIKeyService) ListAPIKeys(ctx context.Context, projectID string) ([]domain.APIKey, error) {
	return svc.store.ListAPIKeys(ctx, projectID)
}

func (svc *APIKeyService) ListAllAPIKeys(ctx context.Context) ([]domain.APIKey, error) {
	return svc.store.ListAllAPIKeys(ctx)
}

func (svc *APIKeyService) DeleteAPIKey(ctx context.Context, id string) error {
	return svc.store.DeleteAPIKey(ctx, id)
}

func (svc *APIKeyService) ValidAPIKeySHA256(ctx context.Context, keySHA256 string) (projectID string, scope string, found bool, err error) {
	return svc.store.ValidAPIKeySHA256(ctx, keySHA256)
}

func (svc *APIKeyService) TouchAPIKey(ctx context.Context, keySHA256 string) error {
	return svc.store.TouchAPIKey(ctx, keySHA256)
}
