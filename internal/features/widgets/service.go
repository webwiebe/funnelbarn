package widgets

import (
	"context"
	"database/sql"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// Queries is the read side of the widget store.
type Queries interface {
	WidgetByID(ctx context.Context, id string) (Widget, error)
	ListWidgets(ctx context.Context, projectID string) ([]Widget, error)
	WidgetBreakdown(ctx context.Context, projectID, eventName, property string, window, limit int) ([]domain.PropertyBreakdown, error)
}

// Commands is the write side of the widget store.
type Commands interface {
	CreateWidget(ctx context.Context, w Widget) (Widget, error)
	UpdateWidget(ctx context.Context, w Widget) (Widget, error)
	DeleteWidget(ctx context.Context, id string) error
}

// Repo is everything the service needs from storage.
type Repo interface {
	Queries
	Commands
}

var (
	_ Queries = (*ReadStore)(nil)
	_ Repo    = (*Store)(nil)
)

// Service is the application layer of the widget slice.
type Service struct {
	store Repo
}

// NewService returns a widget service over store.
func NewService(store Repo) *Service {
	return &Service{store: store}
}

// New returns a widget service over a store on the write pool db and the
// read pool rdb.
func New(db, rdb *sql.DB) *Service {
	return NewService(NewStore(db, rdb))
}

func (svc *Service) CreateWidget(ctx context.Context, w Widget) (Widget, error) {
	return svc.store.CreateWidget(ctx, w)
}

func (svc *Service) GetWidget(ctx context.Context, id string) (Widget, error) {
	return svc.store.WidgetByID(ctx, id)
}

func (svc *Service) ListWidgets(ctx context.Context, projectID string) ([]Widget, error) {
	return svc.store.ListWidgets(ctx, projectID)
}

func (svc *Service) UpdateWidget(ctx context.Context, w Widget) (Widget, error) {
	return svc.store.UpdateWidget(ctx, w)
}

func (svc *Service) DeleteWidget(ctx context.Context, id string) error {
	return svc.store.DeleteWidget(ctx, id)
}

func (svc *Service) WidgetBreakdown(ctx context.Context, projectID, eventName, property string, window, limit int) ([]domain.PropertyBreakdown, error) {
	return svc.store.WidgetBreakdown(ctx, projectID, eventName, property, window, limit)
}
