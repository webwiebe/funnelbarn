package segments

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/httpx"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// API is what the segment handlers need from the service. The funnel
// handlers and the MCP server read segments through it as well.
type API interface {
	CreateSegment(ctx context.Context, projectID, name string, rules []domain.SegmentRule) (Segment, error)
	ListSegments(ctx context.Context, projectID string) ([]Segment, error)
	GetSegment(ctx context.Context, id string) (Segment, error)
	UpdateSegment(ctx context.Context, id, name string, rules []domain.SegmentRule) (Segment, error)
	DeleteSegment(ctx context.Context, id string) error
}

var _ API = (*Service)(nil)

// Handler serves the segment routes. The API server registers them behind its
// session middleware.
type Handler struct {
	svc API
}

// NewHandler returns the segment handlers over svc.
func NewHandler(svc API) *Handler {
	return &Handler{svc: svc}
}

// API returns the service the handlers run on, for the funnel handlers and the
// MCP server that read segments too.
func (h *Handler) API() API {
	return h.svc
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		httpx.JSONError(w, "project id required", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "segments.list",
		attribute.String("project.id", projectID),
	)
	defer span.End()

	segs, err := h.svc.ListSegments(ctx, projectID)
	if err != nil {
		tracing.RecordError(span, err)
		httpx.MapServiceError(w, err, "handleListSegments")
		return
	}
	if segs == nil {
		segs = []Segment{}
	}
	span.SetAttributes(attribute.Int("segments.count", len(segs)))
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"segments": segs})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		httpx.JSONError(w, "project id required", http.StatusBadRequest)
		return
	}
	var body struct {
		Name  string               `json:"name"`
		Rules []domain.SegmentRule `json:"rules"`
	}
	if err := httpx.ReadJSON(r, &body); err != nil {
		httpx.JSONError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		httpx.JSONError(w, "name is required", http.StatusUnprocessableEntity)
		return
	}
	if body.Rules == nil {
		body.Rules = []domain.SegmentRule{}
	}

	ctx, span := tracing.StartSpan(r.Context(), "segments.create",
		attribute.String("project.id", projectID),
		attribute.String("segment.name", body.Name),
		attribute.Int("segment.rules.count", len(body.Rules)),
	)
	defer span.End()

	seg, err := h.svc.CreateSegment(ctx, projectID, body.Name, body.Rules)
	if err != nil {
		tracing.RecordError(span, err)
		httpx.MapServiceError(w, err, "handleCreateSegment")
		return
	}
	span.SetAttributes(attribute.String("segment.id", seg.ID))
	httpx.WriteJSON(w, http.StatusCreated, seg)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	segID := r.PathValue("sid")
	if projectID == "" || segID == "" {
		httpx.JSONError(w, "project id and segment id required", http.StatusBadRequest)
		return
	}

	// Verify the segment belongs to this project.
	existing, err := h.svc.GetSegment(r.Context(), segID)
	if err != nil {
		httpx.MapServiceError(w, err, "handleUpdateSegment.getSegment")
		return
	}
	if existing.ProjectID != projectID {
		httpx.JSONError(w, "segment not found", http.StatusNotFound)
		return
	}

	var body struct {
		Name  string               `json:"name"`
		Rules []domain.SegmentRule `json:"rules"`
	}
	if err := httpx.ReadJSON(r, &body); err != nil {
		httpx.JSONError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Rules == nil {
		body.Rules = []domain.SegmentRule{}
	}

	ctx, span := tracing.StartSpan(r.Context(), "segments.update",
		attribute.String("project.id", projectID),
		attribute.String("segment.id", segID),
		attribute.Int("segment.rules.count", len(body.Rules)),
	)
	defer span.End()

	seg, err := h.svc.UpdateSegment(ctx, segID, body.Name, body.Rules)
	if err != nil {
		tracing.RecordError(span, err)
		httpx.MapServiceError(w, err, "handleUpdateSegment")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, seg)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	segID := r.PathValue("sid")
	if projectID == "" || segID == "" {
		httpx.JSONError(w, "project id and segment id required", http.StatusBadRequest)
		return
	}

	// Verify the segment belongs to this project.
	existing, err := h.svc.GetSegment(r.Context(), segID)
	if err != nil {
		httpx.MapServiceError(w, err, "handleDeleteSegment.getSegment")
		return
	}
	if existing.ProjectID != projectID {
		httpx.JSONError(w, "segment not found", http.StatusNotFound)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "segments.delete",
		attribute.String("project.id", projectID),
		attribute.String("segment.id", segID),
	)
	defer span.End()

	if err := h.svc.DeleteSegment(ctx, segID); err != nil {
		tracing.RecordError(span, err)
		httpx.MapServiceError(w, err, "handleDeleteSegment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
