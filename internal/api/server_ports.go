package api

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// DistributionRepo provides session field distribution data.
type DistributionRepo interface {
	SessionDistributions(ctx context.Context, projectID string) (map[string][]repository.DistributionEntry, error)
}

// InstanceSettingsRepo is the narrow interface for reading/writing instance-level settings.
type InstanceSettingsRepo = ports.InstanceSettingsRepo

// GeoAnonymizer can zero out geo fields on sessions.
type GeoAnonymizer interface {
	AnonymizeSessionGeo(ctx context.Context, sessionID string) error
	AnonymizeSessionsByIP(ctx context.Context, ip string) (int64, error)
}
