package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

func (s *Server) handleListFlags(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	flags, err := s.flags.ListFlags(r.Context(), projectID)
	if err != nil {
		mapServiceError(w, err, "handleListFlags")
		return
	}
	if flags == nil {
		flags = []domain.FeatureFlag{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"flags": flags})
}

func (s *Server) handleCreateFlag(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	var body service.FlagRequest
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}

	// Required fields, flag_type/targeting_rules defaults, targeting rule
	// syntax and flag_kind validation all live in FlagService, so every
	// creation path (dashboard, API token, MCP) enforces them the same way. A
	// validation failure comes back as a *domain.ValidationError, which
	// mapServiceError turns into a 422 with the message as given.
	flag, err := s.flags.CreateFromRequest(r.Context(), projectID, body)
	if err != nil {
		mapServiceError(w, err, "handleCreateFlag")
		return
	}
	slog.InfoContext(r.Context(), "flag created", "flag_id", flag.ID, "project_id", projectID)
	writeJSON(w, http.StatusCreated, flag)
}

// flagInProject loads the flag named by {fid} and confirms it belongs to the
// project named by {id}. Without this, a project-scoped API token could reach
// any flag in the instance by ID and the URL's project would be decoration —
// the scoping these routes advertise has to be real.
func (s *Server) flagInProject(w http.ResponseWriter, r *http.Request, op string) (domain.FeatureFlag, bool) {
	projectID := r.PathValue("id")
	flagID := r.PathValue("fid")
	if flagID == "" {
		jsonError(w, "flag id required", http.StatusBadRequest)
		return domain.FeatureFlag{}, false
	}
	flag, err := s.flags.GetFlag(r.Context(), flagID)
	if err != nil {
		mapServiceError(w, err, op)
		return domain.FeatureFlag{}, false
	}
	// 404 rather than 403: a caller scoped to another project should not learn
	// that this flag ID exists at all.
	if projectID != "" && flag.ProjectID != projectID {
		jsonError(w, "flag not found", http.StatusNotFound)
		return domain.FeatureFlag{}, false
	}
	return flag, true
}

func (s *Server) handleGetFlag(w http.ResponseWriter, r *http.Request) {
	flag, ok := s.flagInProject(w, r, "handleGetFlag")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, flag)
}

func (s *Server) handleUpdateFlag(w http.ResponseWriter, r *http.Request) {
	existing, ok := s.flagInProject(w, r, "handleUpdateFlag.get")
	if !ok {
		return
	}
	var body service.FlagRequest
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	// Targeting rule syntax and flag_kind validation live in
	// FlagService.PatchFlag, same as create; a bad merged value comes
	// back as a *domain.ValidationError, which mapServiceError turns into 422.
	flag, err := s.flags.PatchFlag(r.Context(), existing, body)
	if err != nil {
		mapServiceError(w, err, "handleUpdateFlag")
		return
	}
	writeJSON(w, http.StatusOK, flag)
}

func (s *Server) handleDeleteFlag(w http.ResponseWriter, r *http.Request) {
	flag, ok := s.flagInProject(w, r, "handleDeleteFlag.get")
	if !ok {
		return
	}
	if err := s.flags.DeleteFlag(r.Context(), flag.ID); err != nil {
		mapServiceError(w, err, "handleDeleteFlag")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFlagAnalysis(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id and flag id required", http.StatusBadRequest)
		return
	}
	flag, ok := s.flagInProject(w, r, "handleFlagAnalysis.getFlag")
	if !ok {
		return
	}
	flagID := flag.ID

	to := time.Now().UTC()
	from := to.AddDate(0, 0, -30)
	switch r.URL.Query().Get("range") {
	case "24h":
		from = to.Add(-24 * time.Hour)
	case "7d":
		from = to.AddDate(0, 0, -7)
	case "30d":
		from = to.AddDate(0, 0, -30)
	}

	ctx, span := tracing.StartSpan(r.Context(), "flags.analysis",
		attribute.String("project.id", projectID),
		attribute.String("flag.id", flagID),
		attribute.String("flag.key", flag.FlagKey),
	)
	defer span.End()

	// A config flag records no evaluations, so a variant/conversion report for
	// it would be an empty table dressed up as a result. Say so instead.
	if flag.Kind == domain.FlagKindConfig {
		span.SetAttributes(attribute.String("flag.kind", flag.Kind))
		writeJSON(w, http.StatusOK, map[string]any{
			"flag":        flag,
			"results":     []domain.FlagAnalysisResult{},
			"significant": false,
			"z_score":     0,
			"from":        from.Format(time.RFC3339),
			"to":          to.Format(time.RFC3339),
			"unavailable": "config",
		})
		return
	}

	results, err := s.flags.AnalyzeFlag(ctx, flag, from, to)
	if err != nil {
		tracing.RecordError(span, err)
		mapServiceError(w, err, "handleFlagAnalysis")
		return
	}
	if results == nil {
		results = []domain.FlagAnalysisResult{}
	}

	// Run z-test if we have exactly 2 variants.
	var zScore float64
	var significant bool
	if len(results) == 2 {
		zScore, significant = service.ZTestTwoProportions(
			results[0].Sample, results[0].Conversions,
			results[1].Sample, results[1].Conversions,
		)
	}

	span.SetAttributes(
		attribute.Int("flag.variant_count", len(results)),
		attribute.Bool("flag.significant", significant),
	)

	writeJSON(w, http.StatusOK, map[string]any{
		"flag":        flag,
		"results":     results,
		"significant": significant,
		"z_score":     zScore,
		"from":        from.Format(time.RFC3339),
		"to":          to.Format(time.RFC3339),
	})
}

func (s *Server) handleEvaluateFlag(w http.ResponseWriter, r *http.Request) {
	// The slow-evaluate clock starts here: the API key lookup is a SQLite read,
	// and in the layer7 disk stalls of 2026-10-29 it held requests for up to
	// 29 s while the evaluation itself took under a millisecond.
	timing := s.startEvalTiming()
	// One wait sum for the whole request, so the API key touch, the health mark
	// and the evaluation's own commands all count toward the slow-evaluate check.
	ctx, _ := service.WithSubmitWait(r.Context())
	r = r.WithContext(ctx)
	projectID, _, ok := s.ingest.EvaluateAPIKeyProjectScope(r)
	timing.auth = time.Since(timing.start)
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.projectHealth != nil {
		s.markFlagsEvaluated(r.Context(), projectID)
	}
	// SDK callers auto-register unknown flags so they surface in the dashboard.
	s.evaluateFlagInProject(w, r, projectID, true, timing)
}

// handlePlaygroundEvaluateFlag is the session-authed dashboard counterpart of
// handleEvaluateFlag. The project ID comes from the URL instead of the API key.
// The dashboard playground never auto-registers — it's a testing surface, and a
// typo there shouldn't create a flag.
func (s *Server) handlePlaygroundEvaluateFlag(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	s.evaluateFlagInProject(w, r, projectID, false, s.startEvalTiming())
}

func (s *Server) evaluateFlagInProject(w http.ResponseWriter, r *http.Request, projectID string, autoRegister bool, timing evalTiming) {
	readStart := time.Now()
	var body struct {
		FlagKey      string         `json:"flag_key"`
		DefaultValue any            `json:"default_value"`
		Context      map[string]any `json:"context"`
		// Kind declares what this flag is ("experiment" or "config") and is
		// honoured only when the flag is auto-registered on this call. An
		// existing flag keeps whatever the dashboard says.
		Kind string `json:"kind"`
	}
	if err := readJSON(r, &body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	timing.read = time.Since(readStart)
	if body.FlagKey == "" {
		jsonError(w, "flag_key is required", http.StatusUnprocessableEntity)
		return
	}
	if body.Context == nil {
		body.Context = map[string]any{}
	}

	ctx, span := tracing.StartSpan(r.Context(), "flags.assignment",
		attribute.String("project.id", projectID),
		attribute.String("flag.key", body.FlagKey),
		attribute.Bool("flag.auto_register", autoRegister),
	)
	defer span.End()

	evalStart := time.Now()
	ctx, submitWait := service.WithSubmitWait(ctx)
	defer func() {
		timing.eval = time.Since(evalStart)
		metrics.FlagEvaluationDuration.Observe(timing.eval.Seconds())
		s.reportEvaluate(ctx, span, projectID, body.FlagKey, timing.phases(), submitWait.Total(), s.currentReadPoolWait()-timing.poolWait)
	}()

	var result service.FlagEvalResult
	var err error
	if autoRegister {
		result, err = s.flags.EvaluateOrRegisterFlag(ctx, projectID, body.FlagKey, body.Context, body.DefaultValue, s.flagAutoRegisterMax, body.Kind)
	} else {
		result, err = s.flags.EvaluateFlag(ctx, projectID, body.FlagKey, body.Context)
	}
	if err != nil {
		code := service.EvalErrorCodeOf(err)
		errorCode := string(code)
		span.SetAttributes(attribute.String("flag.error_code", errorCode))
		metrics.FlagEvaluations.WithLabelValues("ERROR").Inc()

		// The code decides the level and the span: a missing flag is routine,
		// a project at its cap is worth a warning, anything else (DB failure,
		// JSON corruption) is a real server problem to surface.
		if code.IsFault() {
			tracing.RecordError(span, err)
		}
		slog.Log(ctx, code.LogLevel(), code.LogMessage(),
			"err", err, "handled", !code.IsFault(),
			"project_id", projectID,
			"flag_key", body.FlagKey,
			"error_code", errorCode,
			"request_id", RequestIDFromContext(ctx),
		)
		// An error result is the caller's own default, not a resolved value —
		// never let it be cached.
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{
			"value":                 body.DefaultValue,
			"variant":               "",
			"reason":                "ERROR",
			"flag_key":              body.FlagKey,
			"error_code":            errorCode,
			"error":                 err.Error(),
			"cache_max_age_seconds": 0,
		})
		return
	}

	span.SetAttributes(
		attribute.String("flag.reason", result.Reason),
		attribute.String("flag.variant", result.Variant),
	)
	metrics.FlagEvaluations.WithLabelValues(result.Reason).Inc()

	writeEvaluateResult(w, result, body.DefaultValue)
}

// writeEvaluateResult answers one resolved evaluation.
func writeEvaluateResult(w http.ResponseWriter, result service.FlagEvalResult, callerDefault any) {
	// A stored rule naming a missing variant comes back as a result with reason
	// ERROR. The contract for that reason is to echo the caller's default_value.
	if result.Reason == "ERROR" && callerDefault != nil {
		result.Value = callerDefault
	}

	// One sanctioned polling interval beats N guesses. A config flag holds the
	// same value for everyone, so it is safe to reuse for its advertised
	// max-age; an experiment resolves per targeting key and every read is a
	// data point, so it advertises 0 and must not be cached.
	if result.CacheMaxAgeSeconds > 0 {
		w.Header().Set("Cache-Control", fmt.Sprintf("private, max-age=%d", result.CacheMaxAgeSeconds))
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleFlagContextKeys(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	suggestions, err := s.flags.ContextKeySuggestions(r.Context(), projectID)
	if err != nil {
		mapServiceError(w, err, "handleFlagContextKeys")
		return
	}
	if suggestions == nil {
		suggestions = []domain.ContextKeySuggestion{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions})
}
