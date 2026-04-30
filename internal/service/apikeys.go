package service

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// APIKeyService orchestrates API key operations.
type APIKeyService struct {
	repo ports.APIKeyRepo
}

func NewAPIKeyService(repo ports.APIKeyRepo) *APIKeyService {
	return &APIKeyService{repo: repo}
}

func (svc *APIKeyService) CreateAPIKey(ctx context.Context, name, projectID, keySHA256, scope string) (repository.APIKey, error) {
	return svc.repo.CreateAPIKey(ctx, name, projectID, keySHA256, scope)
}

func (svc *APIKeyService) ListAPIKeys(ctx context.Context, projectID string) ([]repository.APIKey, error) {
	return svc.repo.ListAPIKeys(ctx, projectID)
}

func (svc *APIKeyService) ListAllAPIKeys(ctx context.Context) ([]repository.APIKey, error) {
	return svc.repo.ListAllAPIKeys(ctx)
}

func (svc *APIKeyService) DeleteAPIKey(ctx context.Context, id string) error {
	return svc.repo.DeleteAPIKey(ctx, id)
}

func (svc *APIKeyService) ValidAPIKeySHA256(ctx context.Context, keySHA256 string) (projectID string, scope string, found bool, err error) {
	return svc.repo.ValidAPIKeySHA256(ctx, keySHA256)
}

func (svc *APIKeyService) TouchAPIKey(ctx context.Context, keySHA256 string) error {
	return svc.repo.TouchAPIKey(ctx, keySHA256)
}
