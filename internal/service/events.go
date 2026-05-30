package service

import (
	"context"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// EventService handles event business logic.
type EventService struct {
	store ports.EventRepo
}

// NewEventService creates a new EventService.
func NewEventService(store ports.EventRepo) *EventService {
	return &EventService{store: store}
}

func (svc *EventService) InsertEvent(ctx context.Context, e repository.Event) error {
	return svc.store.InsertEvent(ctx, e)
}

func (svc *EventService) ListEvents(ctx context.Context, projectID string, limit, offset int) ([]repository.Event, error) {
	return svc.store.ListEvents(ctx, projectID, limit, offset)
}

func (svc *EventService) CountEvents(ctx context.Context, projectID string, from, to time.Time, env string) (int64, error) {
	return svc.store.CountEvents(ctx, projectID, from, to, env)
}

func (svc *EventService) TopPages(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]repository.PageStat, error) {
	return svc.store.TopPages(ctx, projectID, from, to, limit, env)
}

func (svc *EventService) TopReferrers(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]repository.ReferrerStat, error) {
	return svc.store.TopReferrers(ctx, projectID, from, to, limit, env)
}

func (svc *EventService) DailyEventCounts(ctx context.Context, projectID string, from, to time.Time, env string) ([]repository.TimeSeriesPoint, error) {
	return svc.store.DailyEventCounts(ctx, projectID, from, to, env)
}

func (svc *EventService) HourlyEventCounts(ctx context.Context, projectID string, from, to time.Time, env string) ([]repository.TimeSeriesPoint, error) {
	return svc.store.HourlyEventCounts(ctx, projectID, from, to, env)
}

func (svc *EventService) DailyUniqueSessions(ctx context.Context, projectID string, from, to time.Time, env string) ([]repository.TimeSeriesPoint, error) {
	return svc.store.DailyUniqueSessions(ctx, projectID, from, to, env)
}

func (svc *EventService) TopBrowsers(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]repository.BrowserStat, error) {
	return svc.store.TopBrowsers(ctx, projectID, from, to, limit, env)
}

func (svc *EventService) TopDeviceTypes(ctx context.Context, projectID string, from, to time.Time, env string) ([]repository.DeviceStat, error) {
	return svc.store.TopDeviceTypes(ctx, projectID, from, to, env)
}

func (svc *EventService) TopEventNames(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]repository.EventNameStat, error) {
	return svc.store.TopEventNames(ctx, projectID, from, to, limit, env)
}

func (svc *EventService) TopUTMSources(ctx context.Context, projectID string, from, to time.Time, limit int, env string) ([]repository.UTMStat, error) {
	return svc.store.TopUTMSources(ctx, projectID, from, to, limit, env)
}

func (svc *EventService) BounceRate(ctx context.Context, projectID string, from, to time.Time, env string) (float64, error) {
	return svc.store.BounceRate(ctx, projectID, from, to, env)
}

func (svc *EventService) AvgEventsPerSession(ctx context.Context, projectID string, from, to time.Time, env string) (float64, error) {
	return svc.store.AvgEventsPerSession(ctx, projectID, from, to, env)
}

func (svc *EventService) UniqueSessionCount(ctx context.Context, projectID string, from, to time.Time, env string) (int64, error) {
	return svc.store.UniqueSessionCount(ctx, projectID, from, to, env)
}

func (svc *EventService) GetEventByIngestID(ctx context.Context, ingestID string) (*repository.Event, error) {
	return svc.store.GetEventByIngestID(ctx, ingestID)
}

func (svc *EventService) DistinctEventNames(ctx context.Context, projectID string) ([]string, error) {
	return svc.store.DistinctEventNames(ctx, projectID)
}

func (svc *EventService) DistinctEventProperties(ctx context.Context, projectID, eventName string) ([]string, error) {
	return svc.store.DistinctEventProperties(ctx, projectID, eventName)
}

func (svc *EventService) DistinctPropertyValues(ctx context.Context, projectID, eventName, property string, limit int) ([]string, error) {
	return svc.store.DistinctPropertyValues(ctx, projectID, eventName, property, limit)
}

func (svc *EventService) PopulatedMetadataColumns(ctx context.Context, projectID, eventName string) ([]string, error) {
	return svc.store.PopulatedMetadataColumns(ctx, projectID, eventName)
}

func (svc *EventService) PageFlows(ctx context.Context, projectID, page string, depth int, from, to time.Time, env string) (repository.PageFlowResult, error) {
	return svc.store.PageFlows(ctx, projectID, page, depth, from, to, env)
}

func (svc *EventService) DistinctEnvironments(ctx context.Context, projectID string) ([]string, error) {
	return svc.store.DistinctEnvironments(ctx, projectID)
}
