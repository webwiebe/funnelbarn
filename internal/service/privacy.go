package service

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/ports"
)

// privacyService erases geo data from stored sessions, by session or by IP.
type privacyService struct {
	repo ports.GeoAnonymizerRepo
}

// NewPrivacyService creates the Privacy service.
func NewPrivacyService(repo ports.GeoAnonymizerRepo) Privacy {
	return &privacyService{repo: repo}
}

func (s *privacyService) AnonymizeSessionGeo(ctx context.Context, sessionID string) error {
	return s.repo.AnonymizeSessionGeo(ctx, sessionID)
}

func (s *privacyService) AnonymizeSessionsByIP(ctx context.Context, ip string) (int64, error) {
	return s.repo.AnonymizeSessionsByIP(ctx, ip)
}
