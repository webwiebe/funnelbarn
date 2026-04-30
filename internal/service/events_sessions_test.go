package service_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

func randID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// ---------------------------------------------------------------------------
// Helpers shared across tests in this file
// ---------------------------------------------------------------------------

func makeEvent(projectID, sessionID, name string) repository.Event {
	return repository.Event{
		ID:          randID(),
		ProjectID:   projectID,
		SessionID:   sessionID,
		Name:        name,
		URL:         "https://example.com/" + name,
		IngestID:    "ingest-" + name + "-" + sessionID,
		OccurredAt:  time.Now().UTC(),
		ReferrerDomain: "google.com",
		Browser:     "Chrome",
		DeviceType:  "desktop",
	}
}

func makeSession(projectID, sessionID string) repository.Session {
	return repository.Session{
		ID:          sessionID,
		ProjectID:   projectID,
		FirstSeenAt: time.Now().UTC(),
		LastSeenAt:  time.Now().UTC(),
		EntryURL:    "https://example.com/",
		ExitURL:     "https://example.com/",
	}
}

// ---------------------------------------------------------------------------
// EventService
// ---------------------------------------------------------------------------

func TestEventService_InsertAndList(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	eventSvc := service.NewEventService(store)

	p, err := projSvc.CreateProject(ctx, "Events Site", "eventssite")
	require.NoError(t, err)

	e := makeEvent(p.ID, "sess-a", "page_view")
	require.NoError(t, eventSvc.InsertEvent(ctx, e))

	events, err := eventSvc.ListEvents(ctx, p.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "page_view", events[0].Name)
}

func TestEventService_CountEvents(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	eventSvc := service.NewEventService(store)

	p, _ := projSvc.CreateProject(ctx, "Count Site", "countsite")

	for i := 0; i < 3; i++ {
		require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s1", "click")))
	}

	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	n, err := eventSvc.CountEvents(ctx, p.ID, from, to)
	require.NoError(t, err)
	require.EqualValues(t, 3, n)
}

func TestEventService_UniqueSessionCount(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	eventSvc := service.NewEventService(store)

	p, _ := projSvc.CreateProject(ctx, "Sess Count", "sesscount")
	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s-1", "pv")))
	require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s-2", "pv")))
	require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s-1", "click"))) // same session

	n, err := eventSvc.UniqueSessionCount(ctx, p.ID, from, to)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
}

func TestEventService_TopPages(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	eventSvc := service.NewEventService(store)

	p, _ := projSvc.CreateProject(ctx, "Top Pages", "toppages")
	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s1", "page_view")))
	require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s2", "page_view")))

	pages, err := eventSvc.TopPages(ctx, p.ID, from, to, 10)
	require.NoError(t, err)
	require.NotEmpty(t, pages)
}

func TestEventService_TopReferrers(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	eventSvc := service.NewEventService(store)

	p, _ := projSvc.CreateProject(ctx, "Top Refs", "toprefs")
	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s1", "pv")))

	refs, err := eventSvc.TopReferrers(ctx, p.ID, from, to, 10)
	require.NoError(t, err)
	_ = refs // may be empty or not — just no error
}

func TestEventService_DailyEventCounts(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	eventSvc := service.NewEventService(store)

	p, _ := projSvc.CreateProject(ctx, "Daily EC", "dailyec")
	require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s1", "page_view")))

	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	pts, err := eventSvc.DailyEventCounts(ctx, p.ID, from, to)
	require.NoError(t, err)
	require.Len(t, pts, 1)
}

func TestEventService_DailyUniqueSessions(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	eventSvc := service.NewEventService(store)

	p, _ := projSvc.CreateProject(ctx, "Daily US", "dailyus")
	require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s1", "page_view")))
	require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s2", "page_view")))

	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	pts, err := eventSvc.DailyUniqueSessions(ctx, p.ID, from, to)
	require.NoError(t, err)
	require.Len(t, pts, 1)
	require.EqualValues(t, 2, pts[0].Count)
}

func TestEventService_TopBrowsers(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	eventSvc := service.NewEventService(store)

	p, _ := projSvc.CreateProject(ctx, "Browsers", "browsers")
	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s1", "pv")))

	browsers, err := eventSvc.TopBrowsers(ctx, p.ID, from, to, 5)
	require.NoError(t, err)
	_ = browsers
}

func TestEventService_TopDeviceTypes(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	eventSvc := service.NewEventService(store)

	p, _ := projSvc.CreateProject(ctx, "Devices", "devices")
	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s1", "pv")))

	devs, err := eventSvc.TopDeviceTypes(ctx, p.ID, from, to)
	require.NoError(t, err)
	_ = devs
}

func TestEventService_TopEventNames(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	eventSvc := service.NewEventService(store)

	p, _ := projSvc.CreateProject(ctx, "EventNames", "eventnames")
	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s1", "signup")))
	require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s1", "signup")))

	names, err := eventSvc.TopEventNames(ctx, p.ID, from, to, 10)
	require.NoError(t, err)
	require.NotEmpty(t, names)
	require.Equal(t, "signup", names[0].Name)
}

func TestEventService_TopUTMSources(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	eventSvc := service.NewEventService(store)

	p, _ := projSvc.CreateProject(ctx, "UTM", "utm")
	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s1", "pv")))

	srcs, err := eventSvc.TopUTMSources(ctx, p.ID, from, to, 5)
	require.NoError(t, err)
	_ = srcs
}

func TestEventService_BounceRate(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	eventSvc := service.NewEventService(store)

	p, _ := projSvc.CreateProject(ctx, "Bounce", "bounce")
	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	rate, err := eventSvc.BounceRate(ctx, p.ID, from, to)
	require.NoError(t, err)
	require.GreaterOrEqual(t, rate, float64(0))
}

func TestEventService_AvgEventsPerSession(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	eventSvc := service.NewEventService(store)

	p, _ := projSvc.CreateProject(ctx, "AvgEvt", "avgevt")
	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s1", "pv")))
	require.NoError(t, eventSvc.InsertEvent(ctx, makeEvent(p.ID, "s1", "click")))

	avg, err := eventSvc.AvgEventsPerSession(ctx, p.ID, from, to)
	require.NoError(t, err)
	require.GreaterOrEqual(t, avg, float64(0))
}

func TestEventService_GetEventByIngestID(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	eventSvc := service.NewEventService(store)

	p, _ := projSvc.CreateProject(ctx, "IngestLookup", "ingestlookup")

	e := makeEvent(p.ID, "s1", "purchase")
	require.NoError(t, eventSvc.InsertEvent(ctx, e))

	got, err := eventSvc.GetEventByIngestID(ctx, e.IngestID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, e.IngestID, got.IngestID)

	// Non-existent ingest ID → nil, no error.
	notFound, err := eventSvc.GetEventByIngestID(ctx, "does-not-exist")
	require.NoError(t, err)
	require.Nil(t, notFound)
}

// ---------------------------------------------------------------------------
// SessionService
// ---------------------------------------------------------------------------

func TestSessionService_UpsertAndList(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	sessSvc := service.NewSessionService(store)

	p, _ := projSvc.CreateProject(ctx, "SessSvc", "sesssvc")

	sess := makeSession(p.ID, "sess-1")
	require.NoError(t, sessSvc.UpsertSession(ctx, sess))

	sessions, err := sessSvc.ListSessions(ctx, p.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, "sess-1", sessions[0].ID)
}

func TestSessionService_ActiveSessionCount(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	sessSvc := service.NewSessionService(store)

	p, _ := projSvc.CreateProject(ctx, "Active", "active")

	// Insert a session with a recent last_seen_at.
	sess := repository.Session{
		ID:          "active-1",
		ProjectID:   p.ID,
		FirstSeenAt: time.Now().UTC().Add(-2 * time.Minute),
		LastSeenAt:  time.Now().UTC().Add(-1 * time.Minute),
		EntryURL:    "https://example.com/",
	}
	require.NoError(t, sessSvc.UpsertSession(ctx, sess))

	n, err := sessSvc.ActiveSessionCount(ctx, p.ID, 5)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
}

func TestSessionService_SessionByID(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	sessSvc := service.NewSessionService(store)

	p, _ := projSvc.CreateProject(ctx, "SessById", "sessbyid")

	sess := makeSession(p.ID, "sess-lookup")
	require.NoError(t, sessSvc.UpsertSession(ctx, sess))

	got, err := sessSvc.SessionByID(ctx, "sess-lookup")
	require.NoError(t, err)
	require.Equal(t, "sess-lookup", got.ID)
	require.Equal(t, p.ID, got.ProjectID)
}

// ---------------------------------------------------------------------------
// APIKeyService
// ---------------------------------------------------------------------------

func TestAPIKeyService_CRUD(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	keySvc := service.NewAPIKeyService(store)

	p, _ := projSvc.CreateProject(ctx, "KeySvc", "keysvc")

	const hash = "hash9999999999999999999999999999999999999999999999999999999999999"

	key, err := keySvc.CreateAPIKey(ctx, "my-key", p.ID, hash, "ingest")
	require.NoError(t, err)
	require.NotEmpty(t, key.ID)
	require.Equal(t, "my-key", key.Name)
	require.Equal(t, "ingest", key.Scope)

	// List by project.
	keys, err := keySvc.ListAPIKeys(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, keys, 1)

	// List all.
	all, err := keySvc.ListAllAPIKeys(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, all)

	// Validate SHA256.
	projectID, scope, found, err := keySvc.ValidAPIKeySHA256(ctx, hash)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, p.ID, projectID)
	require.Equal(t, "ingest", scope)

	// Touch last_used_at.
	require.NoError(t, keySvc.TouchAPIKey(ctx, hash))

	// Delete.
	require.NoError(t, keySvc.DeleteAPIKey(ctx, key.ID))
	keys, err = keySvc.ListAPIKeys(ctx, p.ID)
	require.NoError(t, err)
	require.Empty(t, keys)
}

// ---------------------------------------------------------------------------
// ProjectService — remaining gaps
// ---------------------------------------------------------------------------

func TestProjectService_GetProjectBySlug(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	p, _ := svc.CreateProject(ctx, "Slug Site", "slug-site")

	got, err := svc.GetProjectBySlug(ctx, "slug-site")
	require.NoError(t, err)
	require.Equal(t, p.ID, got.ID)
}

func TestProjectService_EnsureProject(t *testing.T) {
	ctx := context.Background()
	svc := service.NewProjectService(newTestStore(t))

	// First call: creates project.
	p, err := svc.EnsureProject(ctx, "auto-slug")
	require.NoError(t, err)
	require.NotEmpty(t, p.ID)

	// Second call: returns existing project.
	p2, err := svc.EnsureProject(ctx, "auto-slug")
	require.NoError(t, err)
	require.Equal(t, p.ID, p2.ID)
}

func TestProjectService_UserByUsername(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	svc := service.NewProjectService(store)

	require.NoError(t, store.UpsertUser(ctx, "testuser", "$2b$hash"))

	u, err := svc.UserByUsername(ctx, "testuser")
	require.NoError(t, err)
	require.Equal(t, "testuser", u.Username)
}

// ---------------------------------------------------------------------------
// ABTestService.AnalyzeABTest and FunnelService.AnalyzeFunnel
// ---------------------------------------------------------------------------

func TestABTestService_AnalyzeABTest(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	abtestSvc := service.NewABTestService(store)

	p, _ := projSvc.CreateProject(ctx, "ABAnalyze", "abanalyze")
	test, err := abtestSvc.CreateABTest(ctx, repository.ABTest{
		ProjectID:       p.ID,
		Name:            "Button Colour",
		Status:          "running",
		ConversionEvent: "purchase",
	})
	require.NoError(t, err)

	from := time.Now().UTC().Add(-24 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	results, err := abtestSvc.AnalyzeABTest(ctx, test, from, to)
	require.NoError(t, err)
	_ = results // may be empty with no events, just no error
}

func TestFunnelService_AnalyzeFunnel(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	funnelSvc := service.NewFunnelService(store)

	p, _ := projSvc.CreateProject(ctx, "FunnelAnalyze", "funnelanalyze")
	f, err := funnelSvc.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Checkout",
		Steps: []repository.FunnelStep{
			{EventName: "cart_view"},
			{EventName: "checkout"},
		},
	})
	require.NoError(t, err)

	from := time.Now().UTC().Add(-24 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	results, err := funnelSvc.AnalyzeFunnel(ctx, f, from, to, nil)
	require.NoError(t, err)
	_ = results
}
