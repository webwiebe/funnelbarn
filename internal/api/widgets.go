package api

import (
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

func (s *Server) handleListWidgets(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "widgets.list",
		attribute.String("project.id", projectID),
	)
	defer span.End()

	widgets, err := s.widgets.ListWidgets(ctx, projectID)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleListWidgets")
		return
	}
	if widgets == nil {
		widgets = []repository.DashboardWidget{}
	}
	span.SetAttributes(attribute.Int("widgets.count", len(widgets)))
	writeJSON(w, http.StatusOK, map[string]any{"widgets": widgets})
}

func (s *Server) handleCreateWidget(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	var body struct {
		EventName string `json:"event_name"`
		Property  string `json:"property"`
		Title     string `json:"title"`
		Position  int    `json:"position"`
		Size      int    `json:"size"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.EventName == "" {
		jsonError(w, "event_name is required", http.StatusUnprocessableEntity)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "widgets.create",
		attribute.String("project.id", projectID),
		attribute.String("event.name", body.EventName),
		attribute.String("widget.property", body.Property),
	)
	defer span.End()

	widget, err := s.widgets.CreateWidget(ctx, repository.DashboardWidget{
		ProjectID: projectID,
		EventName: body.EventName,
		Property:  body.Property,
		Title:     body.Title,
		Position:  body.Position,
		Size:      body.Size,
	})
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleCreateWidget")
		return
	}
	span.SetAttributes(attribute.String("widget.id", widget.ID))
	writeJSON(w, http.StatusCreated, widget)
}

func (s *Server) handleUpdateWidget(w http.ResponseWriter, r *http.Request) {
	widgetID := r.PathValue("wid")
	if widgetID == "" {
		jsonError(w, "widget id required", http.StatusBadRequest)
		return
	}

	var body struct {
		EventName string `json:"event_name"`
		Property  string `json:"property"`
		Title     string `json:"title"`
		Position  int    `json:"position"`
		Size      int    `json:"size"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "widgets.update",
		attribute.String("widget.id", widgetID),
		attribute.String("event.name", body.EventName),
		attribute.String("widget.property", body.Property),
	)
	defer span.End()

	widget, err := s.widgets.UpdateWidget(ctx, repository.DashboardWidget{
		ID:        widgetID,
		EventName: body.EventName,
		Property:  body.Property,
		Title:     body.Title,
		Position:  body.Position,
		Size:      body.Size,
	})
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleUpdateWidget")
		return
	}
	writeJSON(w, http.StatusOK, widget)
}

func (s *Server) handleDeleteWidget(w http.ResponseWriter, r *http.Request) {
	widgetID := r.PathValue("wid")
	if widgetID == "" {
		jsonError(w, "widget id required", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "widgets.delete",
		attribute.String("widget.id", widgetID),
	)
	defer span.End()

	if err := s.widgets.DeleteWidget(ctx, widgetID); err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleDeleteWidget")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWidgetBreakdown(w http.ResponseWriter, r *http.Request) {
	widgetID := r.PathValue("wid")
	if widgetID == "" {
		jsonError(w, "widget id required", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "widgets.breakdown",
		attribute.String("widget.id", widgetID),
	)
	defer span.End()

	widget, err := s.widgets.GetWidget(ctx, widgetID)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleWidgetBreakdown")
		return
	}

	window := 100
	limit := 10
	if v := r.URL.Query().Get("window"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			window = n
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			limit = n
		}
	}
	span.SetAttributes(
		attribute.String("project.id", widget.ProjectID),
		attribute.String("event.name", widget.EventName),
		attribute.Int("window", window),
		attribute.Int("limit", limit),
	)

	breakdown, err := s.widgets.WidgetBreakdown(ctx, widget.ProjectID, widget.EventName, widget.Property, window, limit)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleWidgetBreakdown")
		return
	}
	if breakdown == nil {
		breakdown = []repository.PropertyBreakdown{}
	}
	span.SetAttributes(attribute.Int("breakdown.count", len(breakdown)))
	writeJSON(w, http.StatusOK, map[string]any{
		"widget":    widget,
		"breakdown": breakdown,
		"window":    window,
	})
}

func (s *Server) handleBatchBreakdowns(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "widgets.batch_breakdowns",
		attribute.String("project.id", projectID),
	)
	defer span.End()

	widgets, err := s.widgets.ListWidgets(ctx, projectID)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleBatchBreakdowns")
		return
	}

	span.SetAttributes(attribute.Int("widgets.count", len(widgets)))

	type widgetResult struct {
		Widget    repository.DashboardWidget     `json:"widget"`
		Breakdown []repository.PropertyBreakdown `json:"breakdown"`
	}

	results := make([]widgetResult, 0, len(widgets))
	for _, widget := range widgets {
		bd, err := s.widgets.WidgetBreakdown(ctx, widget.ProjectID, widget.EventName, widget.Property, 100, 10)
		if err != nil {
			bd = []repository.PropertyBreakdown{}
		}
		if bd == nil {
			bd = []repository.PropertyBreakdown{}
		}
		results = append(results, widgetResult{Widget: widget, Breakdown: bd})
	}

	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
