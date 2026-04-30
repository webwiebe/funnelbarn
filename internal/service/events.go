package service

import (
	"context"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// EventService handles event business logic.
type EventService struct {
	store *repository.Store
}

// NewEventService creates a new EventService.
func NewEventService(store *repository.Store) *EventService {
	return &EventService{store: store}
}

func (svc *EventService) InsertEvent(ctx context.Context, e repository.Event) error {
	return svc.store.InsertEvent(ctx, e)
}

func (svc *EventService) ListEvents(ctx context.Context, projectID string, limit, offset int) ([]repository.Event, error) {
	return svc.store.ListEvents(ctx, projectID, limit, offset)
}

func (svc *EventService) CountEvents(ctx context.Context, projectID string, from, to time.Time) (int64, error) {
	return svc.store.CountEvents(ctx, projectID, from, to)
}

func (svc *EventService) TopPages(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.PageStat, error) {
	return svc.store.TopPages(ctx, projectID, from, to, limit)
}

func (svc *EventService) TopReferrers(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.ReferrerStat, error) {
	return svc.store.TopReferrers(ctx, projectID, from, to, limit)
}

func (svc *EventService) DailyEventCounts(ctx context.Context, projectID string, from, to time.Time) ([]repository.TimeSeriesPoint, error) {
	return svc.store.DailyEventCounts(ctx, projectID, from, to)
}

func (svc *EventService) DailyUniqueSessions(ctx context.Context, projectID string, from, to time.Time) ([]repository.TimeSeriesPoint, error) {
	return svc.store.DailyUniqueSessions(ctx, projectID, from, to)
}

func (svc *EventService) TopBrowsers(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.BrowserStat, error) {
	return svc.store.TopBrowsers(ctx, projectID, from, to, limit)
}

func (svc *EventService) TopDeviceTypes(ctx context.Context, projectID string, from, to time.Time) ([]repository.DeviceStat, error) {
	return svc.store.TopDeviceTypes(ctx, projectID, from, to)
}

func (svc *EventService) TopEventNames(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.EventNameStat, error) {
	return svc.store.TopEventNames(ctx, projectID, from, to, limit)
}

func (svc *EventService) TopUTMSources(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.UTMStat, error) {
	return svc.store.TopUTMSources(ctx, projectID, from, to, limit)
}

func (svc *EventService) BounceRate(ctx context.Context, projectID string, from, to time.Time) (float64, error) {
	return svc.store.BounceRate(ctx, projectID, from, to)
}

func (svc *EventService) AvgEventsPerSession(ctx context.Context, projectID string, from, to time.Time) (float64, error) {
	return svc.store.AvgEventsPerSession(ctx, projectID, from, to)
}

func (svc *EventService) UniqueSessionCount(ctx context.Context, projectID string, from, to time.Time) (int64, error) {
	return svc.store.UniqueSessionCount(ctx, projectID, from, to)
}

func (svc *EventService) GetEventByIngestID(ctx context.Context, ingestID string) (*repository.Event, error) {
	return svc.store.GetEventByIngestID(ctx, ingestID)
}
