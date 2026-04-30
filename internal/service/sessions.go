package service

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// SessionService handles session business logic.
type SessionService struct {
	store repository.Querier
}

// NewSessionService creates a new SessionService.
func NewSessionService(store repository.Querier) *SessionService {
	return &SessionService{store: store}
}

func (svc *SessionService) UpsertSession(ctx context.Context, sess repository.Session) error {
	return svc.store.UpsertSession(ctx, sess)
}

func (svc *SessionService) SessionByID(ctx context.Context, id string) (repository.Session, error) {
	return svc.store.SessionByID(ctx, id)
}

func (svc *SessionService) ListSessions(ctx context.Context, projectID string, limit, offset int) ([]repository.Session, error) {
	return svc.store.ListSessions(ctx, projectID, limit, offset)
}

func (svc *SessionService) ActiveSessionCount(ctx context.Context, projectID string, withinMinutes int) (int64, error) {
	return svc.store.ActiveSessionCount(ctx, projectID, withinMinutes)
}
