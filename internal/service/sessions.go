package service

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
)

// SessionService handles session business logic.
type SessionService struct {
	store ports.SessionRepo
}

// NewSessionService creates a new SessionService.
func NewSessionService(store ports.SessionRepo) *SessionService {
	return &SessionService{store: store}
}

func (svc *SessionService) UpsertSession(ctx context.Context, sess domain.Session) error {
	return svc.store.UpsertSession(ctx, sess)
}

func (svc *SessionService) SessionByID(ctx context.Context, projectID, id string) (domain.Session, error) {
	return svc.store.SessionByID(ctx, projectID, id)
}

func (svc *SessionService) ListSessions(ctx context.Context, projectID string, limit, offset int) ([]domain.Session, error) {
	return svc.store.ListSessions(ctx, projectID, limit, offset)
}

func (svc *SessionService) ActiveSessionCount(ctx context.Context, projectID string, withinMinutes int) (int64, error) {
	return svc.store.ActiveSessionCount(ctx, projectID, withinMinutes)
}

func (svc *SessionService) SessionDistributions(ctx context.Context, projectID string) (map[string][]domain.DistributionEntry, error) {
	return svc.store.SessionDistributions(ctx, projectID)
}
