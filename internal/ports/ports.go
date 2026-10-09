// Package ports defines the repository interfaces (dependency inversion layer).
// Services depend on these interfaces. Each aggregate has a Queries port,
// implemented by *repository.ReadStore (read-only pool), and a Commands port,
// implemented by *repository.Store; XRepo composes the two.
//
// Dependency direction: entry points → services → ports ← repository adapters.
// Nothing in this package depends on service or api packages.
package ports

import (
	"context"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// ProjectQueries is the read side of the project port.
type ProjectQueries interface {
	ProjectByID(ctx context.Context, id string) (repository.Project, error)
	ProjectBySlug(ctx context.Context, slug string) (repository.Project, error)
	ListProjects(ctx context.Context) ([]repository.Project, error)
	HasProjects(ctx context.Context) (bool, error)
	UserByUsername(ctx context.Context, username string) (repository.User, error)
}

// ProjectCommands is the write side of the project port.
type ProjectCommands interface {
	CreateProject(ctx context.Context, name, slug string) (repository.Project, error)
	UpdateProject(ctx context.Context, id, name, domain string) (repository.Project, error)
	DeleteProject(ctx context.Context, id string) error
	ApproveProject(ctx context.Context, id string) (repository.Project, error)
	EnsureProject(ctx context.Context, slug string) (repository.Project, error)
	EnsureProjectPending(ctx context.Context, name, slug string) (repository.Project, error)
	EnsureSetupAPIKey(ctx context.Context, projectID, keySHA256 string) error
}

// ProjectRepo is the persistence port for projects and setup operations.
type ProjectRepo interface {
	ProjectQueries
	ProjectCommands
}

// FunnelQueries is the read side of the funnel port.
type FunnelQueries interface {
	FunnelByID(ctx context.Context, id string) (repository.Funnel, error)
	ListFunnels(ctx context.Context, projectID string) ([]repository.Funnel, error)
	AnalyzeFunnel(ctx context.Context, f repository.Funnel, from, to time.Time, seg *repository.SegmentFilter, rules ...repository.SegmentRule) ([]repository.FunnelStepResult, error)
	FunnelSegmentData(ctx context.Context, projectID string) (repository.FunnelSegments, error)
	SessionsAtStep(ctx context.Context, f repository.Funnel, stepOrder int, from, to time.Time, limit int) ([]string, error)
}

// FunnelCommands is the write side of the funnel port.
type FunnelCommands interface {
	CreateFunnel(ctx context.Context, f repository.Funnel) (repository.Funnel, error)
	UpdateFunnel(ctx context.Context, f repository.Funnel) (repository.Funnel, error)
	DeleteFunnel(ctx context.Context, id string) error
}

// FunnelRepo is the persistence port for funnels.
type FunnelRepo interface {
	FunnelQueries
	FunnelCommands
}

// ABTestQueries is the read side of the A/B test port.
type ABTestQueries interface {
	ABTestByID(ctx context.Context, id string) (repository.ABTest, error)
	ListABTests(ctx context.Context, projectID string) ([]repository.ABTest, error)
	AnalyzeABTest(ctx context.Context, t repository.ABTest, from, to time.Time) ([]repository.ABTestResult, error)
}

// ABTestCommands is the write side of the A/B test port.
type ABTestCommands interface {
	CreateABTest(ctx context.Context, t repository.ABTest) (repository.ABTest, error)
}

// ABTestRepo is the persistence port for A/B tests.
type ABTestRepo interface {
	ABTestQueries
	ABTestCommands
}

// FlagQueries is the read side of the feature flag port.
type FlagQueries interface {
	CountAutoFlags(ctx context.Context, projectID string) (int, error)
	FlagByID(ctx context.Context, id string) (repository.FeatureFlag, error)
	FlagByKey(ctx context.Context, projectID, flagKey string) (repository.FeatureFlag, error)
	ListFlags(ctx context.Context, projectID string) ([]repository.FeatureFlag, error)
	FlagContextKeySuggestions(ctx context.Context, projectID string) ([]repository.ContextKeySuggestion, error)
	CountEvaluationsByVariant(ctx context.Context, flagID string, from, to time.Time) (map[string]int64, error)
	CountConversionsByVariant(ctx context.Context, flagID, conversionEvent, projectID string, from, to time.Time) (map[string]int64, error)
}

// FlagCommands is the write side of the feature flag port.
type FlagCommands interface {
	CreateFlag(ctx context.Context, f repository.FeatureFlag) (repository.FeatureFlag, error)
	EnsureAutoFlag(ctx context.Context, f repository.FeatureFlag) (repository.FeatureFlag, error)
	TouchFlagEvaluated(ctx context.Context, flagID string) error
	PurgeStaleAutoFlags(ctx context.Context, cutoff time.Time) (int64, error)
	UpdateFlag(ctx context.Context, f repository.FeatureFlag) (repository.FeatureFlag, error)
	DeleteFlag(ctx context.Context, id string) error
	RecordEvaluation(ctx context.Context, eval repository.FlagEvaluation) error
}

// FlagRepo is the persistence port for feature flags.
type FlagRepo interface {
	FlagQueries
	FlagCommands
}

// EventQueries is the read side of the event port.
type EventQueries interface {
	ListEvents(ctx context.Context, projectID string, limit, offset int) ([]repository.Event, error)
	CountEvents(ctx context.Context, projectID string, from, to time.Time, env string) (int64, error)
	GetEventByIngestID(ctx context.Context, ingestID string) (*repository.Event, error)
	TopPages(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]repository.PageStat, error)
	TopReferrers(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]repository.ReferrerStat, error)
	DailyEventCounts(ctx context.Context, projectID string, from, to time.Time, env string) ([]repository.TimeSeriesPoint, error)
	HourlyEventCounts(ctx context.Context, projectID string, from, to time.Time, env string) ([]repository.TimeSeriesPoint, error)
	DailyUniqueSessions(ctx context.Context, projectID string, from, to time.Time, env string) ([]repository.TimeSeriesPoint, error)
	TopBrowsers(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]repository.BrowserStat, error)
	TopDeviceTypes(ctx context.Context, projectID string, from, to time.Time, env string) ([]repository.DeviceStat, error)
	TopEventNames(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]repository.EventNameStat, error)
	TopUTMSources(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]repository.UTMStat, error)
	BounceRate(ctx context.Context, projectID string, from, to time.Time, env string) (float64, error)
	AvgEventsPerSession(ctx context.Context, projectID string, from, to time.Time, env string) (float64, error)
	UniqueSessionCount(ctx context.Context, projectID string, from, to time.Time, env string) (int64, error)
	DistinctEventNames(ctx context.Context, projectID string) ([]string, error)
	DistinctEventProperties(ctx context.Context, projectID, eventName string) ([]string, error)
	DistinctPropertyValues(ctx context.Context, projectID, eventName, property string, limit int) ([]string, error)
	PopulatedMetadataColumns(ctx context.Context, projectID, eventName string) ([]string, error)
	PageFlows(ctx context.Context, projectID, page string, depth int, from, to time.Time, env string) (repository.PageFlowResult, error)
	DistinctEnvironments(ctx context.Context, projectID string) ([]string, error)
	SessionsForPage(ctx context.Context, projectID, page string, from, to time.Time, limit int) ([]string, error)
}

// EventCommands is the write side of the event port.
type EventCommands interface {
	InsertEvent(ctx context.Context, e repository.Event) error
}

// EventRepo is the persistence port for events and analytics queries.
type EventRepo interface {
	EventQueries
	EventCommands
}

// OverviewQueries is the read side of the cross-project overview port:
// GA-like rollups and the cross-project event list.
type OverviewQueries interface {
	ProjectRollups(ctx context.Context, from, to time.Time, env string) ([]repository.ProjectRollup, error)
	OverviewTotals(ctx context.Context, from, to time.Time, env string) (events, sessions int64, err error)
	OverviewVisitorsByProject(ctx context.Context, from, to time.Time, env string, hourly bool) ([]repository.ProjectDayCount, error)
	OverviewTopPages(ctx context.Context, from, to time.Time, limit int, env string) ([]repository.OverviewPageStat, error)
	OverviewTopReferrers(ctx context.Context, from, to time.Time, limit int, env string) ([]repository.OverviewReferrerStat, error)
	OverviewTopCountries(ctx context.Context, from, to time.Time, limit int, env string) ([]repository.OverviewCountryStat, error)
	OverviewDimensionBreakdown(ctx context.Context, dimension string, from, to time.Time, limit int, env string) ([]repository.DimensionStat, error)
	ListAllEvents(ctx context.Context, f repository.EventFilter, limit int) ([]repository.Event, error)

	// ListProjects supplies the "all projects" set for aggregate analysis.
	ListProjects(ctx context.Context) ([]repository.Project, error)
}

// CanonicalQueries is the read side of the canonical event vocabulary,
// per-project mappings and cross-project canonical funnels.
type CanonicalQueries interface {
	ListCanonicalEvents(ctx context.Context) ([]repository.CanonicalEvent, error)
	CanonicalKeySet(ctx context.Context) (map[string]bool, error)
	ListMappings(ctx context.Context, projectID string) ([]repository.EventNameMapping, error)
	MappingSuggestions(ctx context.Context, projectID string) ([]repository.MappingSuggestion, error)
	ListCanonicalFunnels(ctx context.Context) ([]repository.CanonicalFunnel, error)
	CanonicalFunnelByID(ctx context.Context, id string) (repository.CanonicalFunnel, error)
	AnalyzeCanonicalFunnel(ctx context.Context, f repository.CanonicalFunnel, projectIDs []string, from, to time.Time, seg *repository.SegmentFilter, rules ...repository.SegmentRule) (repository.CanonicalFunnelResult, error)
}

// CanonicalCommands is the write side of the canonical event vocabulary,
// per-project mappings and cross-project canonical funnels.
type CanonicalCommands interface {
	CreateCanonicalEvent(ctx context.Context, c repository.CanonicalEvent) (repository.CanonicalEvent, error)
	UpdateCanonicalEvent(ctx context.Context, c repository.CanonicalEvent) (repository.CanonicalEvent, error)
	DeleteCanonicalEvent(ctx context.Context, key string) error
	UpsertMapping(ctx context.Context, projectID, rawName, canonicalKey string) error
	DeleteMapping(ctx context.Context, projectID, rawName string) error
	CreateCanonicalFunnel(ctx context.Context, f repository.CanonicalFunnel) (repository.CanonicalFunnel, error)
	UpdateCanonicalFunnel(ctx context.Context, f repository.CanonicalFunnel) (repository.CanonicalFunnel, error)
	DeleteCanonicalFunnel(ctx context.Context, id string) error
}

// OverviewRepo is the persistence port for cross-project ("instance-wide")
// analytics: GA-like rollups, the canonical-event vocabulary + mappings, and
// aggregate cross-project funnels.
type OverviewRepo interface {
	OverviewQueries
	CanonicalQueries
	CanonicalCommands
}

// SessionQueries is the read side of the session port.
type SessionQueries interface {
	SessionByID(ctx context.Context, projectID, id string) (repository.Session, error)
	ListSessions(ctx context.Context, projectID string, limit, offset int) ([]repository.Session, error)
	ActiveSessionCount(ctx context.Context, projectID string, withinMinutes int) (int64, error)
}

// SessionCommands is the write side of the session port.
type SessionCommands interface {
	UpsertSession(ctx context.Context, sess repository.Session) error
}

// SessionRepo is the persistence port for sessions.
type SessionRepo interface {
	SessionQueries
	SessionCommands
}

// APIKeyQueries is the read side of the API key port.
type APIKeyQueries interface {
	ListAPIKeys(ctx context.Context, projectID string) ([]repository.APIKey, error)
	ListAllAPIKeys(ctx context.Context) ([]repository.APIKey, error)
	ValidAPIKeySHA256(ctx context.Context, keySHA256 string) (projectID string, scope string, found bool, err error)
}

// APIKeyCommands is the write side of the API key port.
type APIKeyCommands interface {
	CreateAPIKey(ctx context.Context, name, projectID, keySHA256, scope string) (repository.APIKey, error)
	DeleteAPIKey(ctx context.Context, id string) error
	TouchAPIKey(ctx context.Context, keySHA256 string) error
}

// APIKeyRepo is the persistence port for API keys.
type APIKeyRepo interface {
	APIKeyQueries
	APIKeyCommands
}

// WidgetQueries is the read side of the dashboard widget port.
type WidgetQueries interface {
	WidgetByID(ctx context.Context, id string) (repository.DashboardWidget, error)
	ListWidgets(ctx context.Context, projectID string) ([]repository.DashboardWidget, error)
	WidgetBreakdown(ctx context.Context, projectID, eventName, property string, window, limit int) ([]repository.PropertyBreakdown, error)
}

// WidgetCommands is the write side of the dashboard widget port.
type WidgetCommands interface {
	CreateWidget(ctx context.Context, w repository.DashboardWidget) (repository.DashboardWidget, error)
	UpdateWidget(ctx context.Context, w repository.DashboardWidget) (repository.DashboardWidget, error)
	DeleteWidget(ctx context.Context, id string) error
}

// WidgetRepo is the persistence port for dashboard widgets.
type WidgetRepo interface {
	WidgetQueries
	WidgetCommands
}

// SegmentQueries is the read side of the segment port.
type SegmentQueries interface {
	SegmentByID(ctx context.Context, id string) (repository.Segment, error)
	ListSegments(ctx context.Context, projectID string) ([]repository.Segment, error)
}

// SegmentCommands is the write side of the segment port.
type SegmentCommands interface {
	CreateSegment(ctx context.Context, seg repository.Segment) (repository.Segment, error)
	UpdateSegment(ctx context.Context, seg repository.Segment) (repository.Segment, error)
	DeleteSegment(ctx context.Context, id string) error
}

// SegmentRepo is the persistence port for user-defined segments.
type SegmentRepo interface {
	SegmentQueries
	SegmentCommands
}

// RecordingQueries is the read side of the recording port.
type RecordingQueries interface {
	GetRecording(ctx context.Context, id string) (repository.Recording, error)
	ListRecordings(ctx context.Context, projectID string, opts repository.RecordingListOpts) ([]repository.Recording, error)
	ListOldRecordings(ctx context.Context, before time.Time) ([]repository.Recording, error)
	ListBrokenRecordings(ctx context.Context) ([]repository.Recording, error)
	ListBotRecordings(ctx context.Context) ([]repository.Recording, error)
	FlagEvaluationsForSession(ctx context.Context, sessionID, projectID string) ([]repository.FlagEvaluationEntry, error)
	LookupTrace(ctx context.Context, projectID, traceID string) (repository.TraceLookup, bool, error)
	TracesForRecording(ctx context.Context, recordingID string) ([]repository.TraceLink, error)
}

// RecordingCommands is the write side of the recording port.
type RecordingCommands interface {
	UpsertRecording(ctx context.Context, r repository.Recording) error
	DeleteRecording(ctx context.Context, id string) error
	InsertTraceLinks(ctx context.Context, projectID, sessionID, recordingID string, links []repository.TraceLink) error
	ApplyChunk(ctx context.Context, r repository.Recording, chunkIndex int, links []repository.TraceLink) (bool, error)
}

// RecordingRepo is the persistence port for session recordings.
type RecordingRepo interface {
	RecordingQueries
	RecordingCommands
}

// ProjectHealthQueries is the read side of the project health port.
type ProjectHealthQueries interface {
	GetProjectHealth(ctx context.Context, projectID string) (repository.ProjectHealth, error)
}

// ProjectHealthCommands is the write side of the project health port.
type ProjectHealthCommands interface {
	MarkProjectHealthSetupCalled(ctx context.Context, projectID string) error
	MarkProjectHealthEventsReceived(ctx context.Context, projectID string) error
	MarkProjectHealthFlagsEvaluated(ctx context.Context, projectID string) error
	MarkProjectHealthRecordingsReceived(ctx context.Context, projectID string) error
	ResetProjectHealth(ctx context.Context, projectID string) error
}

// ProjectHealthRepo is the persistence port for project integration health.
type ProjectHealthRepo interface {
	ProjectHealthQueries
	ProjectHealthCommands
}

// InstanceSettingsQueries is the read side of the instance settings port.
type InstanceSettingsQueries interface {
	GetInstanceSetting(ctx context.Context, key string) (string, bool, error)
	GetAllInstanceSettings(ctx context.Context) (map[string]string, error)
}

// InstanceSettingsCommands is the write side of the instance settings port.
type InstanceSettingsCommands interface {
	SetInstanceSetting(ctx context.Context, key, value string) error
}

// InstanceSettingsRepo is the persistence port for instance-level settings.
type InstanceSettingsRepo interface {
	InstanceSettingsQueries
	InstanceSettingsCommands
}

// EventPersister is the narrow interface worker.PersistEvent requires.
type EventPersister interface {
	PersistEvent(ctx context.Context, e repository.Event, sess repository.Session, signals *repository.SessionSignals) (bool, error)
}
