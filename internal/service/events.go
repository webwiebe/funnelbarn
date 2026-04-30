package service

import (
	"context"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// EventService provides access to event analytics queries.
type EventService struct {
	repo ports.EventRepo
}

func NewEventService(repo ports.EventRepo) *EventService {
	return &EventService{repo: repo}
}

func (svc *EventService) InsertEvent(ctx context.Context, e repository.Event) error {
	return svc.repo.InsertEvent(ctx, e)
}

func (svc *EventService) ListEvents(ctx context.Context, projectID string, limit, offset int) ([]repository.Event, error) {
	return svc.repo.ListEvents(ctx, projectID, limit, offset)
}

func (svc *EventService) CountEvents(ctx context.Context, projectID string, from, to time.Time) (int64, error) {
	return svc.repo.CountEvents(ctx, projectID, from, to)
}

func (svc *EventService) TopPages(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.PageStat, error) {
	return svc.repo.TopPages(ctx, projectID, from, to, limit)
}

func (svc *EventService) TopReferrers(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.ReferrerStat, error) {
	return svc.repo.TopReferrers(ctx, projectID, from, to, limit)
}

func (svc *EventService) DailyEventCounts(ctx context.Context, projectID string, from, to time.Time) ([]repository.TimeSeriesPoint, error) {
	return svc.repo.DailyEventCounts(ctx, projectID, from, to)
}

func (svc *EventService) DailyUniqueSessions(ctx context.Context, projectID string, from, to time.Time) ([]repository.TimeSeriesPoint, error) {
	return svc.repo.DailyUniqueSessions(ctx, projectID, from, to)
}

func (svc *EventService) TopBrowsers(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.BrowserStat, error) {
	return svc.repo.TopBrowsers(ctx, projectID, from, to, limit)
}

func (svc *EventService) TopDeviceTypes(ctx context.Context, projectID string, from, to time.Time) ([]repository.DeviceStat, error) {
	return svc.repo.TopDeviceTypes(ctx, projectID, from, to)
}

func (svc *EventService) TopEventNames(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.EventNameStat, error) {
	return svc.repo.TopEventNames(ctx, projectID, from, to, limit)
}

func (svc *EventService) TopUTMSources(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.UTMStat, error) {
	return svc.repo.TopUTMSources(ctx, projectID, from, to, limit)
}

func (svc *EventService) BounceRate(ctx context.Context, projectID string, from, to time.Time) (float64, error) {
	return svc.repo.BounceRate(ctx, projectID, from, to)
}

func (svc *EventService) AvgEventsPerSession(ctx context.Context, projectID string, from, to time.Time) (float64, error) {
	return svc.repo.AvgEventsPerSession(ctx, projectID, from, to)
}

func (svc *EventService) UniqueSessionCount(ctx context.Context, projectID string, from, to time.Time) (int64, error) {
	return svc.repo.UniqueSessionCount(ctx, projectID, from, to)
}

func (svc *EventService) GetEventByIngestID(ctx context.Context, ingestID string) (*repository.Event, error) {
	return svc.repo.GetEventByIngestID(ctx, ingestID)
}
