package repository_test

// Repository-level tests for events, sessions, and A/B tests.
// These tests use in-memory SQLite via newTestStore(t).

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func insertEvent(t *testing.T, s *repository.Store, projectID, sessionID, name, url string) repository.Event {
	t.Helper()
	ctx := context.Background()

	e := repository.Event{
		ID:             randomHex(t),
		ProjectID:      projectID,
		SessionID:      sessionID,
		Name:           name,
		URL:            url,
		ReferrerDomain: "google.com",
		Browser:        "Chrome",
		DeviceType:     "desktop",
		IngestID:       "ingest-" + randomHex(t),
		// Store as RFC3339 string so SQLite date functions can parse it.
		OccurredAt: time.Now().UTC(),
	}
	require.NoError(t, s.InsertEvent(ctx, e))
	return e
}

func randomHex(t *testing.T) string {
	t.Helper()
	var b [8]byte
	_, err := fmt.Sscanf(fmt.Sprintf("%d", time.Now().UnixNano()), "%d", new(int64))
	_ = err
	for i := range b {
		b[i] = byte(time.Now().UnixNano()>>uint(i*8)) | byte(i)
	}
	return fmt.Sprintf("%016x", time.Now().UnixNano())
}

func insertSession(t *testing.T, s *repository.Store, projectID, sessionID string, lastSeen time.Time) {
	t.Helper()
	sess := repository.Session{
		ID:          sessionID,
		ProjectID:   projectID,
		FirstSeenAt: lastSeen.Add(-5 * time.Minute),
		LastSeenAt:  lastSeen,
		EntryURL:    "https://example.com/",
		ExitURL:     "https://example.com/page",
	}
	require.NoError(t, s.UpsertSession(context.Background(), sess))
}

// ---------------------------------------------------------------------------
// Events
// ---------------------------------------------------------------------------

func TestInsertEvent_ListEvents(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "EvtRepo", "evtrepo")

	e := insertEvent(t, s, p.ID, "sess-1", "page_view", "https://example.com/")

	events, err := s.ListEvents(ctx, p.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, e.Name, events[0].Name)
}

func TestCountEvents(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "CountEvt", "countevt")

	insertEvent(t, s, p.ID, "s1", "click", "https://example.com/")
	insertEvent(t, s, p.ID, "s2", "click", "https://example.com/")

	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	n, err := s.CountEvents(ctx, p.ID, from, to)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
}

func TestTopPages(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "TopPg", "toppg")

	insertEvent(t, s, p.ID, "s1", "pv", "https://example.com/home")
	insertEvent(t, s, p.ID, "s2", "pv", "https://example.com/home")
	insertEvent(t, s, p.ID, "s3", "pv", "https://example.com/about")

	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	pages, err := s.TopPages(ctx, p.ID, from, to, 10)
	require.NoError(t, err)
	require.NotEmpty(t, pages)
	require.Equal(t, "https://example.com/home", pages[0].URL)
	require.EqualValues(t, 2, pages[0].Views)
}

func TestTopReferrers(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "TopRef", "topref")

	insertEvent(t, s, p.ID, "s1", "pv", "https://example.com/")

	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	refs, err := s.TopReferrers(ctx, p.ID, from, to, 10)
	require.NoError(t, err)
	require.NotEmpty(t, refs)
	require.Equal(t, "google.com", refs[0].Domain)
}

func TestUniqueSessionCount(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "UniqSess", "uniqsess")

	insertEvent(t, s, p.ID, "sess-a", "pv", "https://example.com/")
	insertEvent(t, s, p.ID, "sess-b", "pv", "https://example.com/")
	insertEvent(t, s, p.ID, "sess-a", "click", "https://example.com/btn") // same session

	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	n, err := s.UniqueSessionCount(ctx, p.ID, from, to)
	require.NoError(t, err)
	require.EqualValues(t, 2, n)
}

func TestGetEventByIngestID(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "IngestID", "ingestid")

	e := insertEvent(t, s, p.ID, "sess-1", "signup", "https://example.com/signup")

	got, err := s.GetEventByIngestID(ctx, e.IngestID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, e.IngestID, got.IngestID)

	// Non-existent ingest ID → nil, no error.
	none, err := s.GetEventByIngestID(ctx, "does-not-exist")
	require.NoError(t, err)
	require.Nil(t, none)
}

func TestTopBrowsers(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "Browsers", "browsersrepo")

	insertEvent(t, s, p.ID, "s1", "pv", "https://example.com/")

	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	browsers, err := s.TopBrowsers(ctx, p.ID, from, to, 5)
	require.NoError(t, err)
	_ = browsers
}

func TestTopDeviceTypes(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "DevTypes", "devtypes")

	insertEvent(t, s, p.ID, "s1", "pv", "https://example.com/")

	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	devs, err := s.TopDeviceTypes(ctx, p.ID, from, to)
	require.NoError(t, err)
	_ = devs
}

func TestTopEventNames(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "EvtNames", "evtnames")

	insertEvent(t, s, p.ID, "s1", "signup", "https://example.com/")
	insertEvent(t, s, p.ID, "s2", "signup", "https://example.com/")
	insertEvent(t, s, p.ID, "s3", "login", "https://example.com/")

	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	names, err := s.TopEventNames(ctx, p.ID, from, to, 10)
	require.NoError(t, err)
	require.NotEmpty(t, names)
	require.Equal(t, "signup", names[0].Name)
	require.EqualValues(t, 2, names[0].Count)
}

func TestTopUTMSources(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "UTMSrc", "utmsrc")

	// Insert event with UTM source.
	e := repository.Event{
		ID:         randomHex(t),
		ProjectID:  p.ID,
		SessionID:  "s1",
		Name:       "pv",
		IngestID:   "ingest-utm",
		UTMSource:  "newsletter",
		OccurredAt: time.Now().UTC(),
	}
	require.NoError(t, s.InsertEvent(context.Background(), e))

	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	srcs, err := s.TopUTMSources(ctx, p.ID, from, to, 5)
	require.NoError(t, err)
	require.NotEmpty(t, srcs)
	require.Equal(t, "newsletter", srcs[0].Value)
}

func TestBounceRate(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "BounceRepo", "bouncerepo")

	// One session with 1 event (bounce), one with 2 (non-bounce).
	insertEvent(t, s, p.ID, "single-ev", "pv", "https://example.com/")
	insertEvent(t, s, p.ID, "multi-ev", "pv", "https://example.com/")
	insertEvent(t, s, p.ID, "multi-ev", "click", "https://example.com/btn")

	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	rate, err := s.BounceRate(ctx, p.ID, from, to)
	require.NoError(t, err)
	// 1 out of 2 sessions = 0.5
	require.InDelta(t, 0.5, rate, 0.01)
}

func TestAvgEventsPerSession(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "AvgEvtRepo", "avgevtrepo")

	insertEvent(t, s, p.ID, "s-1", "pv", "https://example.com/")
	insertEvent(t, s, p.ID, "s-1", "click", "https://example.com/")
	insertEvent(t, s, p.ID, "s-2", "pv", "https://example.com/")

	from := time.Now().UTC().Add(-time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	avg, err := s.AvgEventsPerSession(ctx, p.ID, from, to)
	require.NoError(t, err)
	// 3 events / 2 sessions = 1.5
	require.InDelta(t, 1.5, avg, 0.01)
}

func TestDailyEventCounts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "DailyCounts", "dailycounts")

	insertEvent(t, s, p.ID, "s1", "pv", "https://example.com/")
	insertEvent(t, s, p.ID, "s2", "pv", "https://example.com/")

	from := time.Now().UTC().Add(-24 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	pts, err := s.DailyEventCounts(ctx, p.ID, from, to)
	require.NoError(t, err)
	require.NotEmpty(t, pts)
	total := int64(0)
	for _, pt := range pts {
		total += pt.Count
	}
	require.EqualValues(t, 2, total)
}

func TestDailyUniqueSessions(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "DailyUniq", "dailyuniq")

	insertEvent(t, s, p.ID, "sess-a", "pv", "https://example.com/")
	insertEvent(t, s, p.ID, "sess-b", "pv", "https://example.com/")
	insertEvent(t, s, p.ID, "sess-a", "click", "https://example.com/") // same session, same day

	from := time.Now().UTC().Add(-24 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	pts, err := s.DailyUniqueSessions(ctx, p.ID, from, to)
	require.NoError(t, err)
	require.NotEmpty(t, pts)
	// Two unique sessions today.
	require.EqualValues(t, 2, pts[0].Count)
}

func TestListEvents_Pagination(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "Paginate", "paginate")

	for i := 0; i < 5; i++ {
		insertEvent(t, s, p.ID, fmt.Sprintf("sess-%d", i), "pv", "https://example.com/")
		time.Sleep(time.Millisecond) // ensure ordering
	}

	// First page: 3 items.
	page1, err := s.ListEvents(ctx, p.ID, 3, 0)
	require.NoError(t, err)
	require.Len(t, page1, 3)

	// Second page: remaining 2.
	page2, err := s.ListEvents(ctx, p.ID, 3, 3)
	require.NoError(t, err)
	require.Len(t, page2, 2)
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

func TestUpsertSession_ActiveCount(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "SessRepo", "sessrepo")

	now := time.Now().UTC()
	insertSession(t, s, p.ID, "sess-recent", now.Add(-1*time.Minute))
	insertSession(t, s, p.ID, "sess-old", now.Add(-30*time.Minute))

	n, err := s.ActiveSessionCount(ctx, p.ID, 5) // within 5 minutes
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
}

func TestListSessions(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "SessList", "sesslist-repo")

	insertSession(t, s, p.ID, "s-a", time.Now().UTC())
	insertSession(t, s, p.ID, "s-b", time.Now().UTC())

	sessions, err := s.ListSessions(ctx, p.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, sessions, 2)
}

func TestSessionByID(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "SessById", "sessbyid-repo")

	insertSession(t, s, p.ID, "sess-lookup", time.Now().UTC())

	sess, err := s.SessionByID(ctx, "sess-lookup")
	require.NoError(t, err)
	require.Equal(t, "sess-lookup", sess.ID)
}

func TestUpsertSession_UpdatesExisting(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "SessUpdate", "sessupdate")

	first := repository.Session{
		ID:          "s-update",
		ProjectID:   p.ID,
		FirstSeenAt: time.Now().UTC().Add(-10 * time.Minute),
		LastSeenAt:  time.Now().UTC().Add(-10 * time.Minute),
		EntryURL:    "https://example.com/",
		ExitURL:     "https://example.com/",
	}
	require.NoError(t, s.UpsertSession(ctx, first))

	// Upsert again with later last_seen_at.
	second := first
	second.LastSeenAt = time.Now().UTC()
	second.ExitURL = "https://example.com/checkout"
	require.NoError(t, s.UpsertSession(ctx, second))

	// Should still be one session, not two.
	sessions, err := s.ListSessions(ctx, p.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
}

// ---------------------------------------------------------------------------
// A/B Tests
// ---------------------------------------------------------------------------

func TestCreateABTest_ListABTests(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "ABRepo", "abrepo")

	test, err := s.CreateABTest(ctx, repository.ABTest{
		ProjectID:       p.ID,
		Name:            "Headline",
		Status:          "running",
		ConversionEvent: "signup",
	})
	require.NoError(t, err)
	require.NotEmpty(t, test.ID)

	tests, err := s.ListABTests(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, tests, 1)
	require.Equal(t, "Headline", tests[0].Name)
}

func TestABTestByID(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "ABByID", "abbyid")

	test, _ := s.CreateABTest(ctx, repository.ABTest{
		ProjectID:       p.ID,
		Name:            "CTA",
		Status:          "running",
		ConversionEvent: "purchase",
	})

	got, err := s.ABTestByID(ctx, test.ID)
	require.NoError(t, err)
	require.Equal(t, test.ID, got.ID)
	require.Equal(t, "CTA", got.Name)
}

func TestAnalyzeABTest_EmptyReturnsNoError(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "ABAnalyze", "abanalyze")

	test, _ := s.CreateABTest(ctx, repository.ABTest{
		ProjectID:       p.ID,
		Name:            "Button",
		Status:          "running",
		ConversionEvent: "click",
	})

	from := time.Now().UTC().Add(-24 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	results, err := s.AnalyzeABTest(ctx, test, from, to)
	require.NoError(t, err)
	_ = results // empty with no events is fine
}

// ---------------------------------------------------------------------------
// Projects — remaining gaps
// ---------------------------------------------------------------------------

func TestUpdateProject(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, _ := s.CreateProject(ctx, "Old Name", "old-name-repo")

	updated, err := s.UpdateProject(ctx, p.ID, "New Name")
	require.NoError(t, err)
	require.Equal(t, "New Name", updated.Name)
	require.Equal(t, p.ID, updated.ID)
}

func TestHasProjects(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	has, err := s.HasProjects(ctx)
	require.NoError(t, err)
	require.False(t, has)

	_, _ = s.CreateProject(ctx, "First", "first-repo")

	has, err = s.HasProjects(ctx)
	require.NoError(t, err)
	require.True(t, has)
}

func TestEnsureProject(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p1, err := s.EnsureProject(ctx, "auto-created")
	require.NoError(t, err)
	require.NotEmpty(t, p1.ID)

	// Second call returns same project.
	p2, err := s.EnsureProject(ctx, "auto-created")
	require.NoError(t, err)
	require.Equal(t, p1.ID, p2.ID)
}

// ---------------------------------------------------------------------------
// API Keys — remaining gaps
// ---------------------------------------------------------------------------

func TestTouchAPIKey(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "TouchKey", "touchkey")

	const hash = "touch666666666666666666666666666666666666666666666666666666666666"
	_, err := s.CreateAPIKey(ctx, "touchable", p.ID, hash, "ingest")
	require.NoError(t, err)

	require.NoError(t, s.TouchAPIKey(ctx, hash))
}

func TestListAllAPIKeys(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	pa, _ := s.CreateProject(ctx, "PA", "pa-repo")
	pb, _ := s.CreateProject(ctx, "PB", "pb-repo")

	_, err := s.CreateAPIKey(ctx, "k-a", pa.ID, "hash-a-99999999999999999999999999999999999999999999999999999999999999", "ingest")
	require.NoError(t, err)
	_, err = s.CreateAPIKey(ctx, "k-b", pb.ID, "hash-b-99999999999999999999999999999999999999999999999999999999999999", "full")
	require.NoError(t, err)

	all, err := s.ListAllAPIKeys(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(all), 2)
}

func TestEnsureSetupAPIKey(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "SetupKey", "setupkey")

	const hash = "setup888888888888888888888888888888888888888888888888888888888888"
	require.NoError(t, s.EnsureSetupAPIKey(ctx, p.ID, hash))
	// Idempotent.
	require.NoError(t, s.EnsureSetupAPIKey(ctx, p.ID, hash))
}

// ---------------------------------------------------------------------------
// Funnels — remaining gaps
// ---------------------------------------------------------------------------

func TestUpdateFunnel(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "FunnelUpd", "funnelupd")

	f, _ := s.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Old",
		Steps:     []repository.FunnelStep{{EventName: "step-1"}},
	})

	updated, err := s.UpdateFunnel(ctx, repository.Funnel{
		ID:        f.ID,
		ProjectID: p.ID,
		Name:      "New",
		Steps: []repository.FunnelStep{
			{EventName: "step-a"},
			{EventName: "step-b"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "New", updated.Name)
	require.Len(t, updated.Steps, 2)
}

func TestAnalyzeFunnel_EmptyReturnsNoError(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "FunnelAnalysis", "funnelanalysis")

	f, _ := s.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Checkout",
		Steps: []repository.FunnelStep{
			{EventName: "cart"},
			{EventName: "purchase"},
		},
	})

	from := time.Now().UTC().Add(-24 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	results, err := s.AnalyzeFunnel(ctx, f, from, to, nil)
	require.NoError(t, err)
	_ = results
}

func TestFunnelSegmentData(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "FunnelSeg", "funnelseg")

	segs, err := s.FunnelSegmentData(ctx, p.ID)
	require.NoError(t, err)
	require.Empty(t, segs.DeviceTypes) // no events
}

func TestAnalyzeFunnel_WithEqSegment(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, _ := s.CreateProject(ctx, "FunnelEq", "funneleq")

	f, _ := s.CreateFunnel(ctx, repository.Funnel{
		ProjectID: p.ID,
		Name:      "Mobile Funnel",
		Steps:     []repository.FunnelStep{{EventName: "page_view"}},
	})

	from := time.Now().UTC().Add(-24 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)

	// Op "eq" exercises escapeSQLLiteral.
	seg := &repository.SegmentFilter{Field: "device_type", Op: "eq", Value: "mobile"}
	results, err := s.AnalyzeFunnel(ctx, f, from, to, seg)
	require.NoError(t, err)
	_ = results

	// Op "neq" also exercises escapeSQLLiteral.
	seg2 := &repository.SegmentFilter{Field: "device_type", Op: "neq", Value: "tablet"}
	results2, err := s.AnalyzeFunnel(ctx, f, from, to, seg2)
	require.NoError(t, err)
	_ = results2
}
