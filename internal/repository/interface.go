package repository

import (
	"context"
	"time"
)

// Querier is the interface implemented by *Store.
// It enables service-layer tests to use test doubles instead of a real SQLite instance.
type Querier interface {
	// Projects
	CreateProject(ctx context.Context, name, slug string) (Project, error)
	ProjectByID(ctx context.Context, id string) (Project, error)
	ProjectBySlug(ctx context.Context, slug string) (Project, error)
	EnsureProject(ctx context.Context, slug string) (Project, error)
	EnsureProjectPending(ctx context.Context, name, slug string) (Project, error)
	ListProjects(ctx context.Context) ([]Project, error)
	UpdateProject(ctx context.Context, id, name string) (Project, error)
	DeleteProject(ctx context.Context, id string) error
	ApproveProject(ctx context.Context, id string) (Project, error)
	HasProjects(ctx context.Context) (bool, error)
	EnsureSetupAPIKey(ctx context.Context, projectID, keySHA256 string) error

	// Users
	UpsertUser(ctx context.Context, username, passwordHash string) error
	UserByUsername(ctx context.Context, username string) (User, error)

	// API Keys
	CreateAPIKey(ctx context.Context, name, projectID, keySHA256, scope string) (APIKey, error)
	ListAPIKeys(ctx context.Context, projectID string) ([]APIKey, error)
	ListAllAPIKeys(ctx context.Context) ([]APIKey, error)
	DeleteAPIKey(ctx context.Context, id string) error
	ValidAPIKeySHA256(ctx context.Context, keySHA256 string) (projectID string, scope string, found bool, err error)
	TouchAPIKey(ctx context.Context, keySHA256 string) error

	// Funnels
	CreateFunnel(ctx context.Context, f Funnel) (Funnel, error)
	FunnelByID(ctx context.Context, id string) (Funnel, error)
	ListFunnels(ctx context.Context, projectID string) ([]Funnel, error)
	UpdateFunnel(ctx context.Context, f Funnel) (Funnel, error)
	DeleteFunnel(ctx context.Context, id string) error
	AnalyzeFunnel(ctx context.Context, f Funnel, from, to time.Time, seg *SegmentFilter) ([]FunnelStepResult, error)
	FunnelSegmentData(ctx context.Context, projectID string) (FunnelSegments, error)

	// A/B Tests
	CreateABTest(ctx context.Context, t ABTest) (ABTest, error)
	ListABTests(ctx context.Context, projectID string) ([]ABTest, error)
	ABTestByID(ctx context.Context, id string) (ABTest, error)
	AnalyzeABTest(ctx context.Context, t ABTest, from, to time.Time) ([]ABTestResult, error)

	// Sessions
	UpsertSession(ctx context.Context, s Session) error
	SessionByID(ctx context.Context, id string) (Session, error)
	ListSessions(ctx context.Context, projectID string, limit, offset int) ([]Session, error)
	ActiveSessionCount(ctx context.Context, projectID string, withinMinutes int) (int64, error)

	// Events
	InsertEvent(ctx context.Context, e Event) error
	ListEvents(ctx context.Context, projectID string, limit, offset int) ([]Event, error)
	CountEvents(ctx context.Context, projectID string, from, to time.Time) (int64, error)
	TopPages(ctx context.Context, projectID string, from, to time.Time, limit int) ([]PageStat, error)
	TopReferrers(ctx context.Context, projectID string, from, to time.Time, limit int) ([]ReferrerStat, error)
	DailyEventCounts(ctx context.Context, projectID string, from, to time.Time) ([]TimeSeriesPoint, error)
	DailyUniqueSessions(ctx context.Context, projectID string, from, to time.Time) ([]TimeSeriesPoint, error)
	TopBrowsers(ctx context.Context, projectID string, from, to time.Time, limit int) ([]BrowserStat, error)
	TopDeviceTypes(ctx context.Context, projectID string, from, to time.Time) ([]DeviceStat, error)
	TopEventNames(ctx context.Context, projectID string, from, to time.Time, limit int) ([]EventNameStat, error)
	TopUTMSources(ctx context.Context, projectID string, from, to time.Time, limit int) ([]UTMStat, error)
	BounceRate(ctx context.Context, projectID string, from, to time.Time) (float64, error)
	AvgEventsPerSession(ctx context.Context, projectID string, from, to time.Time) (float64, error)
	UniqueSessionCount(ctx context.Context, projectID string, from, to time.Time) (int64, error)
	GetEventByIngestID(ctx context.Context, ingestID string) (*Event, error)
	PurgeOldEvents(ctx context.Context, cutoff time.Time) (int64, error)
}

// compile-time check that *Store satisfies Querier.
var _ Querier = (*Store)(nil)
