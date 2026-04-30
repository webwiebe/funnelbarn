package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

func newTestEventSvc(t *testing.T) (*service.EventService, string) {
	t.Helper()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	p, err := projSvc.CreateProject(context.Background(), "Ev Project", "ev-project-"+t.Name())
	require.NoError(t, err)
	return service.NewEventService(store), p.ID
}

func TestEventService_InsertAndList(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	evSvc := service.NewEventService(store)

	p, err := projSvc.CreateProject(ctx, "Ev Project", "ev-project-insert")
	require.NoError(t, err)

	now := time.Now().UTC()
	e := repository.Event{
		ID:         "ev-001",
		ProjectID:  p.ID,
		SessionID:  "sess-ev-001",
		Name:       "page-view",
		IngestID:   "ingest-001",
		OccurredAt: now,
	}
	err = evSvc.InsertEvent(ctx, e)
	require.NoError(t, err)

	events, err := evSvc.ListEvents(ctx, p.ID, 10, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "page-view", events[0].Name)
}

func TestEventService_CountEvents(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	evSvc := service.NewEventService(store)

	p, err := projSvc.CreateProject(ctx, "Ev Project", "ev-project-count")
	require.NoError(t, err)

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		e := repository.Event{
			ID:         "ev-count-00" + string(rune('1'+i)),
			ProjectID:  p.ID,
			SessionID:  "sess-count",
			Name:       "click",
			IngestID:   "ingest-count-" + string(rune('a'+i)),
			OccurredAt: now,
		}
		err = evSvc.InsertEvent(ctx, e)
		require.NoError(t, err)
	}

	n, err := evSvc.CountEvents(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
}

func TestEventService_TopPages(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	evSvc := service.NewEventService(store)

	p, err := projSvc.CreateProject(ctx, "Ev Project", "ev-project-toppages")
	require.NoError(t, err)

	now := time.Now().UTC()
	e := repository.Event{
		ID:         "ev-pages-001",
		ProjectID:  p.ID,
		SessionID:  "sess-pages",
		Name:       "page-view",
		URL:        "https://example.com/home",
		IngestID:   "ingest-pages-001",
		OccurredAt: now,
	}
	err = evSvc.InsertEvent(ctx, e)
	require.NoError(t, err)

	pages, err := evSvc.TopPages(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	assert.NotEmpty(t, pages)
}

func TestEventService_UniqueSessionCount(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	evSvc := service.NewEventService(store)

	p, err := projSvc.CreateProject(ctx, "Ev Project", "ev-project-uniqsess")
	require.NoError(t, err)

	now := time.Now().UTC()
	for i := 0; i < 2; i++ {
		e := repository.Event{
			ID:         "ev-uniq-00" + string(rune('1'+i)),
			ProjectID:  p.ID,
			SessionID:  "sess-uniq-" + string(rune('a'+i)),
			Name:       "page-view",
			IngestID:   "ingest-uniq-00" + string(rune('1'+i)),
			OccurredAt: now,
		}
		err = evSvc.InsertEvent(ctx, e)
		require.NoError(t, err)
	}

	n, err := evSvc.UniqueSessionCount(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
}

func TestEventService_BounceRate(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	evSvc := service.NewEventService(store)

	p, err := projSvc.CreateProject(ctx, "Ev Project", "ev-project-bounce")
	require.NoError(t, err)

	now := time.Now().UTC()
	// One session with one event = 100% bounce
	e := repository.Event{
		ID:         "ev-bounce-001",
		ProjectID:  p.ID,
		SessionID:  "sess-bounce-001",
		Name:       "page-view",
		IngestID:   "ingest-bounce-001",
		OccurredAt: now,
	}
	err = evSvc.InsertEvent(ctx, e)
	require.NoError(t, err)

	rate, err := evSvc.BounceRate(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1.0, rate)
}

func TestEventService_GetEventByIngestID(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	evSvc := service.NewEventService(store)

	p, err := projSvc.CreateProject(ctx, "Ev Project", "ev-project-ingestid")
	require.NoError(t, err)

	now := time.Now().UTC()
	e := repository.Event{
		ID:         "ev-ingest-001",
		ProjectID:  p.ID,
		SessionID:  "sess-ingest-001",
		Name:       "click",
		IngestID:   "my-unique-ingest-id",
		OccurredAt: now,
	}
	err = evSvc.InsertEvent(ctx, e)
	require.NoError(t, err)

	found, err := evSvc.GetEventByIngestID(ctx, "my-unique-ingest-id")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "click", found.Name)
}

func TestEventService_TopReferrers(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	evSvc := service.NewEventService(store)

	p, err := projSvc.CreateProject(ctx, "Ev Project", "ev-project-referrers")
	require.NoError(t, err)

	now := time.Now().UTC()
	e := repository.Event{
		ID:             "ev-ref-001",
		ProjectID:      p.ID,
		SessionID:      "sess-ref-001",
		Name:           "page-view",
		ReferrerDomain: "twitter.com",
		IngestID:       "ingest-ref-001",
		OccurredAt:     now,
	}
	err = evSvc.InsertEvent(ctx, e)
	require.NoError(t, err)

	refs, err := evSvc.TopReferrers(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	assert.NotEmpty(t, refs)
}

func TestEventService_AvgEventsPerSession(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	evSvc := service.NewEventService(store)

	p, err := projSvc.CreateProject(ctx, "Ev Project", "ev-project-avg")
	require.NoError(t, err)

	now := time.Now().UTC()
	for i := 0; i < 2; i++ {
		e := repository.Event{
			ID:         "ev-avg-00" + string(rune('1'+i)),
			ProjectID:  p.ID,
			SessionID:  "sess-avg-001",
			Name:       "page-view",
			IngestID:   "ingest-avg-00" + string(rune('1'+i)),
			OccurredAt: now,
		}
		err = evSvc.InsertEvent(ctx, e)
		require.NoError(t, err)
	}

	avg, err := evSvc.AvgEventsPerSession(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 2.0, avg)
}

func TestEventService_TopBrowsers(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	evSvc := service.NewEventService(store)

	p, err := projSvc.CreateProject(ctx, "Ev Project", "ev-project-browsers")
	require.NoError(t, err)

	now := time.Now().UTC()
	e := repository.Event{
		ID:         "ev-br-001",
		ProjectID:  p.ID,
		SessionID:  "sess-br-001",
		Name:       "page-view",
		Browser:    "Firefox",
		IngestID:   "ingest-br-001",
		OccurredAt: now,
	}
	err = evSvc.InsertEvent(ctx, e)
	require.NoError(t, err)

	browsers, err := evSvc.TopBrowsers(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	assert.NotEmpty(t, browsers)
}

func TestEventService_TopDeviceTypes(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	evSvc := service.NewEventService(store)

	p, err := projSvc.CreateProject(ctx, "Ev Project", "ev-project-devtypes")
	require.NoError(t, err)

	now := time.Now().UTC()
	e := repository.Event{
		ID:         "ev-dt-001",
		ProjectID:  p.ID,
		SessionID:  "sess-dt-001",
		Name:       "page-view",
		DeviceType: "mobile",
		IngestID:   "ingest-dt-001",
		OccurredAt: now,
	}
	err = evSvc.InsertEvent(ctx, e)
	require.NoError(t, err)

	devices, err := evSvc.TopDeviceTypes(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	assert.NotEmpty(t, devices)
}

func TestEventService_TopEventNames(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	evSvc := service.NewEventService(store)

	p, err := projSvc.CreateProject(ctx, "Ev Project", "ev-project-evnames")
	require.NoError(t, err)

	now := time.Now().UTC()
	e := repository.Event{
		ID:         "ev-names-001",
		ProjectID:  p.ID,
		SessionID:  "sess-names-001",
		Name:       "button-click",
		IngestID:   "ingest-names-001",
		OccurredAt: now,
	}
	err = evSvc.InsertEvent(ctx, e)
	require.NoError(t, err)

	names, err := evSvc.TopEventNames(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	assert.NotEmpty(t, names)
}

func TestEventService_TopUTMSources(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	evSvc := service.NewEventService(store)

	p, err := projSvc.CreateProject(ctx, "Ev Project", "ev-project-utmsrc")
	require.NoError(t, err)

	now := time.Now().UTC()
	e := repository.Event{
		ID:         "ev-utm-001",
		ProjectID:  p.ID,
		SessionID:  "sess-utm-001",
		Name:       "page-view",
		UTMSource:  "newsletter",
		IngestID:   "ingest-utm-001",
		OccurredAt: now,
	}
	err = evSvc.InsertEvent(ctx, e)
	require.NoError(t, err)

	srcs, err := evSvc.TopUTMSources(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour), 10)
	require.NoError(t, err)
	assert.NotEmpty(t, srcs)
}

func TestEventService_DailyEventCounts(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	evSvc := service.NewEventService(store)

	p, err := projSvc.CreateProject(ctx, "Ev Project", "ev-project-dailyev")
	require.NoError(t, err)

	now := time.Now().UTC()
	// No events — verify no error
	_, err = evSvc.DailyEventCounts(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
}

func TestEventService_DailyUniqueSessions(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	projSvc := service.NewProjectService(store)
	evSvc := service.NewEventService(store)

	p, err := projSvc.CreateProject(ctx, "Ev Project", "ev-project-dailyuniq")
	require.NoError(t, err)

	now := time.Now().UTC()
	// No events — verify no error
	_, err = evSvc.DailyUniqueSessions(ctx, p.ID, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
}
