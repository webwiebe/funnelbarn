// Package ports defines the repository interfaces (dependency inversion layer).
// Services depend on these interfaces; *repository.Store satisfies them all.
//
// Dependency direction: entry points → services → ports ← repository adapters.
// Nothing in this package depends on service or api packages.
package ports

import (
	"context"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// ProjectRepo is the persistence port for projects and setup operations.
type ProjectRepo interface {
	CreateProject(ctx context.Context, name, slug string) (repository.Project, error)
	ProjectByID(ctx context.Context, id string) (repository.Project, error)
	ProjectBySlug(ctx context.Context, slug string) (repository.Project, error)
	ListProjects(ctx context.Context) ([]repository.Project, error)
	UpdateProject(ctx context.Context, id, name string) (repository.Project, error)
	DeleteProject(ctx context.Context, id string) error
	ApproveProject(ctx context.Context, id string) (repository.Project, error)
	EnsureProject(ctx context.Context, slug string) (repository.Project, error)
	EnsureProjectPending(ctx context.Context, name, slug string) (repository.Project, error)
	HasProjects(ctx context.Context) (bool, error)
	EnsureSetupAPIKey(ctx context.Context, projectID, keySHA256 string) error
	UserByUsername(ctx context.Context, username string) (repository.User, error)
}

// FunnelRepo is the persistence port for funnels.
type FunnelRepo interface {
	CreateFunnel(ctx context.Context, f repository.Funnel) (repository.Funnel, error)
	FunnelByID(ctx context.Context, id string) (repository.Funnel, error)
	ListFunnels(ctx context.Context, projectID string) ([]repository.Funnel, error)
	UpdateFunnel(ctx context.Context, f repository.Funnel) (repository.Funnel, error)
	DeleteFunnel(ctx context.Context, id string) error
	AnalyzeFunnel(ctx context.Context, f repository.Funnel, from, to time.Time, seg *repository.SegmentFilter) ([]repository.FunnelStepResult, error)
	FunnelSegmentData(ctx context.Context, projectID string) (repository.FunnelSegments, error)
}

// ABTestRepo is the persistence port for A/B tests.
type ABTestRepo interface {
	CreateABTest(ctx context.Context, t repository.ABTest) (repository.ABTest, error)
	ListABTests(ctx context.Context, projectID string) ([]repository.ABTest, error)
	ABTestByID(ctx context.Context, id string) (repository.ABTest, error)
	AnalyzeABTest(ctx context.Context, t repository.ABTest, from, to time.Time) ([]repository.ABTestResult, error)
}

// EventRepo is the persistence port for events and analytics queries.
type EventRepo interface {
	InsertEvent(ctx context.Context, e repository.Event) error
	ListEvents(ctx context.Context, projectID string, limit, offset int) ([]repository.Event, error)
	CountEvents(ctx context.Context, projectID string, from, to time.Time) (int64, error)
	GetEventByIngestID(ctx context.Context, ingestID string) (*repository.Event, error)
	TopPages(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.PageStat, error)
	TopReferrers(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.ReferrerStat, error)
	DailyEventCounts(ctx context.Context, projectID string, from, to time.Time) ([]repository.TimeSeriesPoint, error)
	DailyUniqueSessions(ctx context.Context, projectID string, from, to time.Time) ([]repository.TimeSeriesPoint, error)
	TopBrowsers(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.BrowserStat, error)
	TopDeviceTypes(ctx context.Context, projectID string, from, to time.Time) ([]repository.DeviceStat, error)
	TopEventNames(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.EventNameStat, error)
	TopUTMSources(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.UTMStat, error)
	BounceRate(ctx context.Context, projectID string, from, to time.Time) (float64, error)
	AvgEventsPerSession(ctx context.Context, projectID string, from, to time.Time) (float64, error)
	UniqueSessionCount(ctx context.Context, projectID string, from, to time.Time) (int64, error)
}

// SessionRepo is the persistence port for sessions.
type SessionRepo interface {
	UpsertSession(ctx context.Context, sess repository.Session) error
	SessionByID(ctx context.Context, id string) (repository.Session, error)
	ListSessions(ctx context.Context, projectID string, limit, offset int) ([]repository.Session, error)
	ActiveSessionCount(ctx context.Context, projectID string, withinMinutes int) (int64, error)
}

// APIKeyRepo is the persistence port for API keys.
type APIKeyRepo interface {
	CreateAPIKey(ctx context.Context, name, projectID, keySHA256, scope string) (repository.APIKey, error)
	ListAPIKeys(ctx context.Context, projectID string) ([]repository.APIKey, error)
	ListAllAPIKeys(ctx context.Context) ([]repository.APIKey, error)
	DeleteAPIKey(ctx context.Context, id string) error
	ValidAPIKeySHA256(ctx context.Context, keySHA256 string) (projectID string, scope string, found bool, err error)
	TouchAPIKey(ctx context.Context, keySHA256 string) error
}

// EventPersister is the narrow interface worker.PersistEvent requires.
// It is a subset of EventRepo + SessionRepo to avoid injecting the full store.
type EventPersister interface {
	GetEventByIngestID(ctx context.Context, ingestID string) (*repository.Event, error)
	InsertEvent(ctx context.Context, e repository.Event) error
	UpsertSession(ctx context.Context, sess repository.Session) error
}
