package ports

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// ProjectRecordingSettingsRepo is the persistence port for per-project
// recording settings.
type ProjectRecordingSettingsRepo interface {
	GetProjectRecordingSettings(ctx context.Context, projectID string) (*domain.ProjectRecordingSettings, error)
	UpsertProjectRecordingSettings(ctx context.Context, settings *domain.ProjectRecordingSettings) error
}

// GeoAnonymizerRepo zeroes the geo fields on stored sessions.
type GeoAnonymizerRepo interface {
	AnonymizeSessionGeo(ctx context.Context, sessionID string) error
	AnonymizeSessionsByIP(ctx context.Context, ip string) (int64, error)
}

// WebSessionRepo is the persistence port for the web_sessions table: the
// server-side dashboard sessions. Deleting a row revokes the session.
type WebSessionRepo interface {
	CreateWebSession(ctx context.Context, ws domain.WebSession) error
	GetWebSession(ctx context.Context, idHash string) (domain.WebSession, error)
	UpdateWebSessionTokens(ctx context.Context, idHash, idToken, accessToken, refreshToken string, accessExpiresAt int64, claimsJSON string, lastRefreshAt int64) error
	MarkWebSessionRefreshFailing(ctx context.Context, idHash string, since int64) error
	DeleteWebSession(ctx context.Context, idHash string) error
	DeleteWebSessionsByIdpSid(ctx context.Context, sid string) (int64, error)
	DeleteWebSessionsByIdpSub(ctx context.Context, sub string) (int64, error)
}

// IAMBarnUserRepo is the persistence port the OIDC callback uses to upsert the
// local user record keyed on the stable IdP subject.
type IAMBarnUserRepo interface {
	FindUserByIAMBarnSub(ctx context.Context, sub string) (domain.User, error)
	CreateIAMBarnUser(ctx context.Context, sub, username string) (domain.User, error)
}
