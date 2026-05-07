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

	widgets, err := s.widgets.ListWidgets(r.Context(), projectID)
	if err != nil {
		mapServiceError(w, err, "handleListWidgets")
		return
	}
	if widgets == nil {
		widgets = []repository.DashboardWidget{}
	}
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
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.EventName == "" || body.Property == "" {
		jsonError(w, "event_name and property are required", http.StatusUnprocessableEntity)
		return
	}

	widget, err := s.widgets.CreateWidget(r.Context(), repository.DashboardWidget{
		ProjectID: projectID,
		EventName: body.EventName,
		Property:  body.Property,
		Title:     body.Title,
		Position:  body.Position,
	})
	if err != nil {
		mapServiceError(w, err, "handleCreateWidget")
		return
	}
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
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}

	widget, err := s.widgets.UpdateWidget(r.Context(), repository.DashboardWidget{
		ID:        widgetID,
		EventName: body.EventName,
		Property:  body.Property,
		Title:     body.Title,
		Position:  body.Position,
	})
	if err != nil {
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

	if err := s.widgets.DeleteWidget(r.Context(), widgetID); err != nil {
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

	widget, err := s.widgets.GetWidget(r.Context(), widgetID)
	if err != nil {
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

	breakdown, err := s.widgets.WidgetBreakdown(r.Context(), widget.ProjectID, widget.EventName, widget.Property, window, limit)
	if err != nil {
		mapServiceError(w, err, "handleWidgetBreakdown")
		return
	}
	if breakdown == nil {
		breakdown = []repository.PropertyBreakdown{}
	}
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
