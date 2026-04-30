package service

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// SessionService orchestrates session operations.
type SessionService struct {
	repo ports.SessionRepo
}

func NewSessionService(repo ports.SessionRepo) *SessionService {
	return &SessionService{repo: repo}
}

func (svc *SessionService) UpsertSession(ctx context.Context, sess repository.Session) error {
	return svc.repo.UpsertSession(ctx, sess)
}

func (svc *SessionService) SessionByID(ctx context.Context, id string) (repository.Session, error) {
	return svc.repo.SessionByID(ctx, id)
}

func (svc *SessionService) ListSessions(ctx context.Context, projectID string, limit, offset int) ([]repository.Session, error) {
	return svc.repo.ListSessions(ctx, projectID, limit, offset)
}

func (svc *SessionService) ActiveSessionCount(ctx context.Context, projectID string, withinMinutes int) (int64, error) {
	return svc.repo.ActiveSessionCount(ctx, projectID, withinMinutes)
}
