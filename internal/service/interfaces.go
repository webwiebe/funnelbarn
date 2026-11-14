package service

import (
	"context"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// ProjectHealth is the service interface for tracking integration health per project.
type ProjectHealth interface {
	GetProjectHealth(ctx context.Context, projectID string) (domain.ProjectHealth, error)
	MarkSetupCalled(ctx context.Context, projectID string) error
	// MarkEventsReceived records that at least one event was ingested for
	// projectID. The returned bool is true only on the call that transitions
	// the health flag from false to true in the DB (i.e. this project's first
	// ever ingested event) — a cheap, best-effort "first event" signal reused
	// from the existing integration-health tracking rather than new schema.
	// Under rare concurrent first-request races it may report true more than
	// once for the same project; callers treating it as an activation signal
	// should tolerate that.
	MarkEventsReceived(ctx context.Context, projectID string) (firstTime bool, err error)
	MarkFlagsEvaluated(ctx context.Context, projectID string) error
	MarkRecordingsReceived(ctx context.Context, projectID string) error
	ResetProjectHealth(ctx context.Context, projectID string) error
}

// Projects is the interface for project-related operations.
type Projects interface {
	CreateProject(ctx context.Context, name, slug string) (domain.Project, error)
	ListProjects(ctx context.Context) ([]domain.Project, error)
	GetProject(ctx context.Context, id string) (domain.Project, error)
	GetProjectBySlug(ctx context.Context, slug string) (domain.Project, error)
	UpdateProject(ctx context.Context, id, name, domain string) (domain.Project, error)
	DeleteProject(ctx context.Context, id string) error
	ApproveProject(ctx context.Context, id string) (domain.Project, error)
	EnsureProjectPending(ctx context.Context, name, slug string) (domain.Project, error)
	EnsureProject(ctx context.Context, slug string) (domain.Project, error)
	HasProjects(ctx context.Context) (bool, error)
	UserByUsername(ctx context.Context, username string) (domain.User, error)
}

// Funnels is the interface for funnel-related operations.
type Funnels interface {
	CreateFunnel(ctx context.Context, f domain.Funnel) (domain.Funnel, error)
	ListFunnels(ctx context.Context, projectID string) ([]domain.Funnel, error)
	GetFunnel(ctx context.Context, id string) (domain.Funnel, error)
	UpdateFunnel(ctx context.Context, f domain.Funnel) (domain.Funnel, error)
	DeleteFunnel(ctx context.Context, id string) error
	AnalyzeFunnel(ctx context.Context, f domain.Funnel, from, to time.Time, seg *domain.SegmentFilter, rules ...domain.SegmentRule) ([]domain.FunnelStepResult, error)
	AnalyzeFunnelBreakdown(ctx context.Context, f domain.Funnel, from, to time.Time, seg *domain.SegmentFilter, property string, rules ...domain.SegmentRule) ([]domain.FunnelBreakdownGroup, error)
	FunnelSegmentData(ctx context.Context, projectID string) (domain.FunnelSegments, error)
}

// Flags is the interface for feature flag operations.
type Flags interface {
	CreateFlag(ctx context.Context, f domain.FeatureFlag) (domain.FeatureFlag, error)
	GetFlag(ctx context.Context, id string) (domain.FeatureFlag, error)
	GetFlagByKey(ctx context.Context, projectID, flagKey string) (domain.FeatureFlag, error)
	ListFlags(ctx context.Context, projectID string) ([]domain.FeatureFlag, error)
	UpdateFlag(ctx context.Context, f domain.FeatureFlag) (domain.FeatureFlag, error)
	CreateFromRequest(ctx context.Context, projectID string, req FlagRequest) (domain.FeatureFlag, error)
	PatchFlag(ctx context.Context, existing domain.FeatureFlag, req FlagRequest) (domain.FeatureFlag, error)
	DeleteFlag(ctx context.Context, id string) error
	EvaluateFlag(ctx context.Context, projectID, flagKey string, evalContext map[string]any) (FlagEvalResult, error)
	PreviewFlag(ctx context.Context, projectID, flagKey string, evalContext map[string]any) (FlagEvalResult, error)
	EvaluateOrRegisterFlag(ctx context.Context, projectID, flagKey string, evalContext map[string]any, defaultValue any, maxAuto int, kind string) (FlagEvalResult, error)
	AnalyzeFlag(ctx context.Context, flag domain.FeatureFlag, from, to time.Time) ([]domain.FlagAnalysisResult, error)
	ContextKeySuggestions(ctx context.Context, projectID string) ([]domain.ContextKeySuggestion, error)
	Usage(ctx context.Context, projectID string) (FlagUsageReport, error)
}

// ABTests is the interface for A/B test-related operations.
type ABTests interface {
	CreateABTest(ctx context.Context, t domain.ABTest) (domain.ABTest, error)
	ListABTests(ctx context.Context, projectID string) ([]domain.ABTest, error)
	GetABTest(ctx context.Context, id string) (domain.ABTest, error)
	AnalyzeABTest(ctx context.Context, t domain.ABTest, from, to time.Time) ([]domain.ABTestResult, error)
}

// Events is the interface for event-related operations.
type Events interface {
	InsertEvent(ctx context.Context, e domain.Event) error
	ListEvents(ctx context.Context, projectID string, limit, offset int) ([]domain.Event, error)
	ListEventsFiltered(ctx context.Context, projectID string, f domain.EventListFilter, limit, offset int) ([]domain.Event, error)
	TopEventNamesFiltered(ctx context.Context, projectID string, from, to time.Time, limit int, f domain.EventCountFilter) ([]domain.EventNameStat, error)
	EventBreakdown(ctx context.Context, projectID, eventName, dimension string, from, to time.Time, limit int, f domain.EventCountFilter) ([]domain.BreakdownRow, error)
	CountEvents(ctx context.Context, projectID string, from, to time.Time, env string) (int64, error)
	TopPages(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]domain.PageStat, error)
	TopReferrers(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]domain.ReferrerStat, error)
	DailyEventCounts(ctx context.Context, projectID string, from, to time.Time, env string) ([]domain.TimeSeriesPoint, error)
	HourlyEventCounts(ctx context.Context, projectID string, from, to time.Time, env string) ([]domain.TimeSeriesPoint, error)
	DailyUniqueSessions(ctx context.Context, projectID string, from, to time.Time, env string) ([]domain.TimeSeriesPoint, error)
	TopBrowsers(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]domain.BrowserStat, error)
	TopDeviceTypes(ctx context.Context, projectID string, from, to time.Time, env string) ([]domain.DeviceStat, error)
	TopEventNames(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]domain.EventNameStat, error)
	TopUTMSources(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]domain.UTMStat, error)
	BounceRate(ctx context.Context, projectID string, from, to time.Time, env string) (float64, error)
	AvgEventsPerSession(ctx context.Context, projectID string, from, to time.Time, env string) (float64, error)
	UniqueSessionCount(ctx context.Context, projectID string, from, to time.Time, env string) (int64, error)
	GetEventByIngestID(ctx context.Context, ingestID string) (*domain.Event, error)
	DistinctEventNames(ctx context.Context, projectID string) ([]string, error)
	DistinctEventProperties(ctx context.Context, projectID, eventName string) ([]string, error)
	DistinctPropertyValues(ctx context.Context, projectID, eventName, property string, limit int) ([]string, error)
	PopulatedMetadataColumns(ctx context.Context, projectID, eventName string) ([]string, error)
	PageFlows(ctx context.Context, projectID, page string, depth int, from, to time.Time, env string) (domain.PageFlowResult, error)
	DistinctEnvironments(ctx context.Context, projectID string) ([]string, error)
	SessionsForPage(ctx context.Context, projectID, page string, from, to time.Time) ([]string, error)
}

// Overview is the interface for cross-project ("instance-wide") analytics:
// GA-like rollups, the canonical-event vocabulary + mappings, and aggregate
// cross-project funnels.
type Overview interface {
	ProjectRollups(ctx context.Context, from, to time.Time, env string) ([]domain.ProjectRollup, error)
	OverviewTotals(ctx context.Context, from, to time.Time, env string) (int64, int64, error)
	OverviewVisitorsByProject(ctx context.Context, from, to time.Time, env string, hourly bool) ([]domain.ProjectDayCount, error)
	OverviewTopPages(ctx context.Context, from, to time.Time, limit int, env string) ([]domain.OverviewPageStat, error)
	OverviewTopReferrers(ctx context.Context, from, to time.Time, limit int, env string) ([]domain.OverviewReferrerStat, error)
	OverviewTopCountries(ctx context.Context, from, to time.Time, limit int, env string) ([]domain.OverviewCountryStat, error)
	OverviewDimensionBreakdown(ctx context.Context, dimension string, from, to time.Time, limit int, env string) ([]domain.DimensionStat, error)
	ListAllEvents(ctx context.Context, f domain.EventFilter, limit int) ([]domain.Event, error)

	ListCanonicalEvents(ctx context.Context) ([]domain.CanonicalEvent, error)
	CreateCanonicalEvent(ctx context.Context, c domain.CanonicalEvent) (domain.CanonicalEvent, error)
	UpdateCanonicalEvent(ctx context.Context, c domain.CanonicalEvent) (domain.CanonicalEvent, error)
	DeleteCanonicalEvent(ctx context.Context, key string) error
	ListMappings(ctx context.Context, projectID string) ([]domain.EventNameMapping, error)
	SetMappings(ctx context.Context, projectID string, mappings []domain.EventNameMapping) error
	DeleteMapping(ctx context.Context, projectID, rawName string) error
	MappingSuggestions(ctx context.Context, projectID string) ([]domain.MappingSuggestion, error)

	CreateCanonicalFunnel(ctx context.Context, f domain.CanonicalFunnel) (domain.CanonicalFunnel, error)
	ListCanonicalFunnels(ctx context.Context) ([]domain.CanonicalFunnel, error)
	GetCanonicalFunnel(ctx context.Context, id string) (domain.CanonicalFunnel, error)
	UpdateCanonicalFunnel(ctx context.Context, f domain.CanonicalFunnel) (domain.CanonicalFunnel, error)
	DeleteCanonicalFunnel(ctx context.Context, id string) error
	AnalyzeCanonicalFunnel(ctx context.Context, f domain.CanonicalFunnel, projectIDs []string, from, to time.Time, seg *domain.SegmentFilter, rules ...domain.SegmentRule) (domain.CanonicalFunnelResult, error)
}

// Widgets is the interface for dashboard widget operations.
type Widgets interface {
	CreateWidget(ctx context.Context, w domain.DashboardWidget) (domain.DashboardWidget, error)
	GetWidget(ctx context.Context, id string) (domain.DashboardWidget, error)
	ListWidgets(ctx context.Context, projectID string) ([]domain.DashboardWidget, error)
	UpdateWidget(ctx context.Context, w domain.DashboardWidget) (domain.DashboardWidget, error)
	DeleteWidget(ctx context.Context, id string) error
	WidgetBreakdown(ctx context.Context, projectID, eventName, property string, window, limit int) ([]domain.PropertyBreakdown, error)
}

// Sessions is the interface for session-related operations.
type Sessions interface {
	UpsertSession(ctx context.Context, sess domain.Session) error
	SessionByID(ctx context.Context, projectID, id string) (domain.Session, error)
	ListSessions(ctx context.Context, projectID string, limit, offset int) ([]domain.Session, error)
	ActiveSessionCount(ctx context.Context, projectID string, withinMinutes int) (int64, error)
	SessionDistributions(ctx context.Context, projectID string) (map[string][]domain.DistributionEntry, error)
}

// InstanceSettings is the interface for instance-level settings.
type InstanceSettings interface {
	GetAllInstanceSettings(ctx context.Context) (map[string]string, error)
	SetInstanceSetting(ctx context.Context, key, value string) error
}

// RecordingSettings is the interface for per-project recording settings.
type RecordingSettings interface {
	GetProjectRecordingSettings(ctx context.Context, projectID string) (*domain.ProjectRecordingSettings, error)
	UpsertProjectRecordingSettings(ctx context.Context, settings *domain.ProjectRecordingSettings) error
}

// Privacy is the interface for erasing personal data held on sessions.
type Privacy interface {
	AnonymizeSessionGeo(ctx context.Context, sessionID string) error
	AnonymizeSessionsByIP(ctx context.Context, ip string) (int64, error)
}

// WebSessions is the interface for the server-side dashboard sessions used by
// the session middleware, the OIDC callback and the logout paths.
type WebSessions interface {
	CreateWebSession(ctx context.Context, ws domain.WebSession) error
	GetWebSession(ctx context.Context, idHash string) (domain.WebSession, error)
	UpdateWebSessionTokens(ctx context.Context, idHash, idToken, accessToken, refreshToken string, accessExpiresAt int64, claimsJSON string, lastRefreshAt int64) error
	MarkWebSessionRefreshFailing(ctx context.Context, idHash string, since int64) error
	DeleteWebSession(ctx context.Context, idHash string) error
	DeleteWebSessionsByIdpSid(ctx context.Context, sid string) (int64, error)
	DeleteWebSessionsByIdpSub(ctx context.Context, sub string) (int64, error)
}

// IAMBarnUsers is the interface for the local user records created from an
// IAMBarn login.
type IAMBarnUsers interface {
	FindUserByIAMBarnSub(ctx context.Context, sub string) (domain.User, error)
	CreateIAMBarnUser(ctx context.Context, sub, username string) (domain.User, error)
}

// Segments is the interface for user-defined segment operations.
type Segments interface {
	CreateSegment(ctx context.Context, projectID, name string, rules []domain.SegmentRule) (domain.Segment, error)
	ListSegments(ctx context.Context, projectID string) ([]domain.Segment, error)
	GetSegment(ctx context.Context, id string) (domain.Segment, error)
	UpdateSegment(ctx context.Context, id, name string, rules []domain.SegmentRule) (domain.Segment, error)
	DeleteSegment(ctx context.Context, id string) error
}

// Recordings is the interface for session recording operations.
type Recordings interface {
	IngestChunk(ctx context.Context, chunk RecordingChunk) error
	ListRecordings(ctx context.Context, projectID string, opts domain.RecordingListOpts) ([]domain.Recording, error)
	GetChunk(ctx context.Context, projectID, recordingID string, index int) ([]byte, error)
	GetRecordingSessionID(ctx context.Context, projectID, recordingID string) (string, error)
	FlagEvaluationsForSession(ctx context.Context, projectID, sessionID string) ([]domain.FlagEvaluationEntry, error)
	SessionsAtStep(ctx context.Context, funnelID, projectID string, stepOrder int, from, to time.Time, environment string) ([]string, error)
	SessionsForPage(ctx context.Context, projectID, page string, from, to time.Time) ([]string, error)
	PurgeOldRecordings(ctx context.Context, retentionDays int) error
	PurgeBrokenRecordings(ctx context.Context) (int, error)
	PurgeBotRecordings(ctx context.Context) (int, error)
	PurgeProjectRecordings(ctx context.Context, projectID string) error
	DeleteRecording(ctx context.Context, projectID, recordingID string) error
	LookupTrace(ctx context.Context, projectID, traceID string) (domain.TraceLookup, bool, error)
	TracesForRecording(ctx context.Context, projectID, recordingID string) ([]domain.TraceLink, error)
}

// APIKeys is the interface for API key-related operations.
type APIKeys interface {
	CreateAPIKey(ctx context.Context, name, projectID, keySHA256, scope string) (domain.APIKey, error)
	ListAPIKeys(ctx context.Context, projectID string) ([]domain.APIKey, error)
	ListAllAPIKeys(ctx context.Context) ([]domain.APIKey, error)
	DeleteAPIKey(ctx context.Context, id string) error
	ValidAPIKeySHA256(ctx context.Context, keySHA256 string) (projectID string, scope string, found bool, err error)
	TouchAPIKey(ctx context.Context, keySHA256 string) error
}

// Agents reports the agent fleet's delivery lead times and scoreboard from
// the agents project (#331).
type Agents interface {
	LeadTimes(ctx context.Context, projectID string, from, to time.Time, repo string) (AgentLeadTimes, error)
	Scoreboard(ctx context.Context, projectID string, from, to time.Time, session string) (AgentScoreboard, error)
}
