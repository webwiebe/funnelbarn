package abtests

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/httpx"
	"github.com/wiebe-xyz/funnelbarn/internal/stats"
	"github.com/wiebe-xyz/funnelbarn/internal/timerange"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// API is what the A/B test handlers need from the service.
type API interface {
	CreateABTest(ctx context.Context, t ABTest) (ABTest, error)
	ListABTests(ctx context.Context, projectID string) ([]ABTest, error)
	GetABTest(ctx context.Context, id string) (ABTest, error)
	AnalyzeABTest(ctx context.Context, t ABTest, from, to time.Time) ([]ABTestResult, error)
}

var _ API = (*Service)(nil)

// Handler serves the A/B test routes. The API server registers them behind its
// session middleware.
type Handler struct {
	svc       API
	requestID func(context.Context) string
}

// NewHandler returns the A/B test handlers over svc. requestID reads the
// request id the API server stores in the context, for the creation log line.
func NewHandler(svc API, requestID func(context.Context) string) *Handler {
	return &Handler{svc: svc, requestID: requestID}
}

// List returns all A/B tests for a project.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		httpx.JSONError(w, "project id required", http.StatusBadRequest)
		return
	}

	tests, err := h.svc.ListABTests(r.Context(), projectID)
	if err != nil {
		httpx.MapServiceError(w, err, "handleListABTests")
		return
	}
	if tests == nil {
		tests = []ABTest{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"tests": tests})
}

// Create creates a new A/B test for a project.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		httpx.JSONError(w, "project id required", http.StatusBadRequest)
		return
	}

	var body struct {
		Name            string `json:"name"`
		ControlFilter   string `json:"control_filter"`
		VariantFilter   string `json:"variant_filter"`
		ConversionEvent string `json:"conversion_event"`
	}
	if err := httpx.ReadJSON(r, &body); err != nil {
		httpx.JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "abtests.create",
		attribute.String("project.id", projectID),
		attribute.String("abtest.conversion_event", body.ConversionEvent),
	)
	defer span.End()

	test, err := h.svc.CreateABTest(ctx, ABTest{
		ProjectID:       projectID,
		Name:            body.Name,
		Status:          "running",
		ControlFilter:   body.ControlFilter,
		VariantFilter:   body.VariantFilter,
		ConversionEvent: body.ConversionEvent,
	})
	if err != nil {
		tracing.RecordError(span, err)
		httpx.MapServiceError(w, err, "handleCreateABTest")
		return
	}
	span.SetAttributes(attribute.String("abtest.id", test.ID))
	slog.InfoContext(ctx, "ab test created", "test_id", test.ID, "project_id", projectID, "request_id", h.requestID(ctx))
	httpx.WriteJSON(w, http.StatusCreated, test)
}

// splitABResults maps the per-variant results into the flat control and
// variant totals the UI expects.
func splitABResults(results []ABTestResult) (controlSample, controlConversions, variantSample, variantConversions int64) {
	for _, r := range results {
		switch r.Variant {
		case "control":
			controlSample = r.Total
			controlConversions = r.Conversions
		case "variant":
			variantSample = r.Total
			variantConversions = r.Conversions
		}
	}
	return
}

// Analysis computes variant conversion rates for an A/B test.
func (h *Handler) Analysis(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	testID := r.PathValue("abid")
	if projectID == "" || testID == "" {
		httpx.JSONError(w, "project id and ab test id required", http.StatusBadRequest)
		return
	}

	test, err := h.svc.GetABTest(r.Context(), testID)
	if err != nil {
		httpx.MapServiceError(w, err, "handleABTestAnalysis.getABTest")
		return
	}
	if test.ProjectID != projectID {
		httpx.JSONError(w, "a/b test not found", http.StatusNotFound)
		return
	}

	win := timerange.Parse(r.URL.Query())
	from, to := win.From, win.To

	ctx, span := tracing.StartSpan(r.Context(), "abtests.analyze",
		attribute.String("project.id", test.ProjectID),
		attribute.String("abtest.id", test.ID),
		attribute.String("abtest.conversion_event", test.ConversionEvent),
	)
	defer span.End()
	results, err := h.svc.AnalyzeABTest(ctx, test, from, to)
	if err != nil {
		tracing.RecordError(span, err)
		httpx.MapServiceError(w, err, "handleABTestAnalysis")
		return
	}

	controlSample, controlConversions, variantSample, variantConversions := splitABResults(results)

	// Two-proportion z-test for statistical significance.
	zScore, significant := stats.ZTestTwoProportions(controlSample, controlConversions, variantSample, variantConversions)

	span.SetAttributes(
		attribute.Int64("abtest.control_sample", controlSample),
		attribute.Int64("abtest.variant_sample", variantSample),
		attribute.Bool("abtest.significant", significant),
	)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"test":                test,
		"control_sample":      controlSample,
		"control_conversions": controlConversions,
		"variant_sample":      variantSample,
		"variant_conversions": variantConversions,
		"significant":         significant,
		"z_score":             zScore,
		"from":                from.Format(time.RFC3339),
		"to":                  to.Format(time.RFC3339),
	})
}
