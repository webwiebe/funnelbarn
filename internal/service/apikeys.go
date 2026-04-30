package service

import (
	"context"
	"strings"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// APIKeyService handles API key business logic.
type APIKeyService struct {
	store repository.Querier
}

// NewAPIKeyService creates a new APIKeyService.
func NewAPIKeyService(store repository.Querier) *APIKeyService {
	return &APIKeyService{store: store}
}

func (svc *APIKeyService) CreateAPIKey(ctx context.Context, name, projectID, keySHA256, scope string) (repository.APIKey, error) {
	if strings.TrimSpace(name) == "" {
		return repository.APIKey{}, &domain.ValidationError{Field: "name", Message: "required"}
	}
	if strings.TrimSpace(scope) == "" {
		return repository.APIKey{}, &domain.ValidationError{Field: "scope", Message: "required"}
	}
	return svc.store.CreateAPIKey(ctx, name, projectID, keySHA256, scope)
}

func (svc *APIKeyService) ListAPIKeys(ctx context.Context, projectID string) ([]repository.APIKey, error) {
	return svc.store.ListAPIKeys(ctx, projectID)
}

func (svc *APIKeyService) ListAllAPIKeys(ctx context.Context) ([]repository.APIKey, error) {
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
