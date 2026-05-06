package service

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

type WidgetService struct {
	store repository.Querier
}

func NewWidgetService(store repository.Querier) *WidgetService {
	return &WidgetService{store: store}
}

func (svc *WidgetService) CreateWidget(ctx context.Context, w repository.DashboardWidget) (repository.DashboardWidget, error) {
	return svc.store.CreateWidget(ctx, w)
}

func (svc *WidgetService) GetWidget(ctx context.Context, id string) (repository.DashboardWidget, error) {
	return svc.store.WidgetByID(ctx, id)
}

func (svc *WidgetService) ListWidgets(ctx context.Context, projectID string) ([]repository.DashboardWidget, error) {
	return svc.store.ListWidgets(ctx, projectID)
}

func (svc *WidgetService) UpdateWidget(ctx context.Context, w repository.DashboardWidget) (repository.DashboardWidget, error) {
	return svc.store.UpdateWidget(ctx, w)
}

func (svc *WidgetService) DeleteWidget(ctx context.Context, id string) error {
	return svc.store.DeleteWidget(ctx, id)
}

func (svc *WidgetService) WidgetBreakdown(ctx context.Context, projectID, eventName, property string, window, limit int) ([]repository.PropertyBreakdown, error) {
	return svc.store.WidgetBreakdown(ctx, projectID, eventName, property, window, limit)
}
