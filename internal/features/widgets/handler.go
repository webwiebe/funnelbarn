package widgets

import (
	"context"
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/httpx"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// API is what the widget handlers need from the service.
type API interface {
	CreateWidget(ctx context.Context, w Widget) (Widget, error)
	GetWidget(ctx context.Context, id string) (Widget, error)
	ListWidgets(ctx context.Context, projectID string) ([]Widget, error)
	UpdateWidget(ctx context.Context, w Widget) (Widget, error)
	DeleteWidget(ctx context.Context, id string) error
	WidgetBreakdown(ctx context.Context, projectID, eventName, property string, window, limit int) ([]domain.PropertyBreakdown, error)
}

var _ API = (*Service)(nil)

// Handler serves the widget routes. The API server registers them behind its
// session middleware.
type Handler struct {
	svc API
}

// NewHandler returns the widget handlers over svc.
func NewHandler(svc API) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		httpx.JSONError(w, "project id required", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "widgets.list",
		attribute.String("project.id", projectID),
	)
	defer span.End()

	widgets, err := h.svc.ListWidgets(ctx, projectID)
	if err != nil {
		tracing.RecordError(span, err)
		httpx.MapServiceError(w, err, "handleListWidgets")
		return
	}
	if widgets == nil {
		widgets = []Widget{}
	}
	span.SetAttributes(attribute.Int("widgets.count", len(widgets)))
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"widgets": widgets})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		httpx.JSONError(w, "project id required", http.StatusBadRequest)
		return
	}

	var body struct {
		EventName string `json:"event_name"`
		Property  string `json:"property"`
		Title     string `json:"title"`
		Position  int    `json:"position"`
		Size      int    `json:"size"`
	}
	if err := httpx.ReadJSON(r, &body); err != nil {
		httpx.JSONError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.EventName == "" {
		httpx.JSONError(w, "event_name is required", http.StatusUnprocessableEntity)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "widgets.create",
		attribute.String("project.id", projectID),
		attribute.String("event.name", body.EventName),
		attribute.String("widget.property", body.Property),
	)
	defer span.End()

	widget, err := h.svc.CreateWidget(ctx, Widget{
		ProjectID: projectID,
		EventName: body.EventName,
		Property:  body.Property,
		Title:     body.Title,
		Position:  body.Position,
		Size:      body.Size,
	})
	if err != nil {
		tracing.RecordError(span, err)
		httpx.MapServiceError(w, err, "handleCreateWidget")
		return
	}
	span.SetAttributes(attribute.String("widget.id", widget.ID))
	httpx.WriteJSON(w, http.StatusCreated, widget)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	widgetID := r.PathValue("wid")
	if widgetID == "" {
		httpx.JSONError(w, "widget id required", http.StatusBadRequest)
		return
	}

	var body struct {
		EventName string `json:"event_name"`
		Property  string `json:"property"`
		Title     string `json:"title"`
		Position  int    `json:"position"`
		Size      int    `json:"size"`
	}
	if err := httpx.ReadJSON(r, &body); err != nil {
		httpx.JSONError(w, "invalid json", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "widgets.update",
		attribute.String("widget.id", widgetID),
		attribute.String("event.name", body.EventName),
		attribute.String("widget.property", body.Property),
	)
	defer span.End()

	widget, err := h.svc.UpdateWidget(ctx, Widget{
		ID:        widgetID,
		EventName: body.EventName,
		Property:  body.Property,
		Title:     body.Title,
		Position:  body.Position,
		Size:      body.Size,
	})
	if err != nil {
		tracing.RecordError(span, err)
		httpx.MapServiceError(w, err, "handleUpdateWidget")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, widget)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	widgetID := r.PathValue("wid")
	if widgetID == "" {
		httpx.JSONError(w, "widget id required", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "widgets.delete",
		attribute.String("widget.id", widgetID),
	)
	defer span.End()

	if err := h.svc.DeleteWidget(ctx, widgetID); err != nil {
		tracing.RecordError(span, err)
		httpx.MapServiceError(w, err, "handleDeleteWidget")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Breakdown(w http.ResponseWriter, r *http.Request) {
	widgetID := r.PathValue("wid")
	if widgetID == "" {
		httpx.JSONError(w, "widget id required", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "widgets.breakdown",
		attribute.String("widget.id", widgetID),
	)
	defer span.End()

	widget, err := h.svc.GetWidget(ctx, widgetID)
	if err != nil {
		tracing.RecordError(span, err)
		httpx.MapServiceError(w, err, "handleWidgetBreakdown")
		return
	}

	window := positiveQueryInt(r, "window", 100, 1000)
	limit := positiveQueryInt(r, "limit", 10, 50)
	span.SetAttributes(
		attribute.String("project.id", widget.ProjectID),
		attribute.String("event.name", widget.EventName),
		attribute.Int("window", window),
		attribute.Int("limit", limit),
	)

	breakdown, err := h.svc.WidgetBreakdown(ctx, widget.ProjectID, widget.EventName, widget.Property, window, limit)
	if err != nil {
		tracing.RecordError(span, err)
		httpx.MapServiceError(w, err, "handleWidgetBreakdown")
		return
	}
	if breakdown == nil {
		breakdown = []domain.PropertyBreakdown{}
	}
	span.SetAttributes(attribute.Int("breakdown.count", len(breakdown)))
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"widget":    widget,
		"breakdown": breakdown,
		"window":    window,
	})
}

// positiveQueryInt reads a query parameter between 1 and most, and returns def
// when it is missing or out of range.
func positiveQueryInt(r *http.Request, name string, def, most int) int {
	n, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil || n < 1 || n > most {
		return def
	}
	return n
}

func (h *Handler) BatchBreakdowns(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		httpx.JSONError(w, "project id required", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "widgets.batch_breakdowns",
		attribute.String("project.id", projectID),
	)
	defer span.End()

	widgets, err := h.svc.ListWidgets(ctx, projectID)
	if err != nil {
		tracing.RecordError(span, err)
		httpx.MapServiceError(w, err, "handleBatchBreakdowns")
		return
	}

	span.SetAttributes(attribute.Int("widgets.count", len(widgets)))

	type widgetResult struct {
		Widget    Widget                     `json:"widget"`
		Breakdown []domain.PropertyBreakdown `json:"breakdown"`
	}

	results := make([]widgetResult, 0, len(widgets))
	for _, widget := range widgets {
		bd, err := h.svc.WidgetBreakdown(ctx, widget.ProjectID, widget.EventName, widget.Property, 100, 10)
		if err != nil {
			bd = []domain.PropertyBreakdown{}
		}
		if bd == nil {
			bd = []domain.PropertyBreakdown{}
		}
		results = append(results, widgetResult{Widget: widget, Breakdown: bd})
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": results})
}
