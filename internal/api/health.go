package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// healthPingTimeout bounds the liveness/readiness database ping. It must stay
// below the probes' timeoutSeconds in deploy/k8s/*/deployment.yaml, so that the
// prober waits for this handler's verdict instead of pre-empting it.
const healthPingTimeout = 2 * time.Second

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Ping on a context detached from the caller's cancellation. kube-probe
	// defaults to timeoutSeconds: 1 and the probes did not set it, so any ping
	// slower than a second was cancelled by the prober before this handler's
	// own budget expired. Ping then returned context.Canceled, which was logged
	// at Error and filed as a BugBarn issue whose entire message was "context
	// canceled" — the prober's clock, reported as a database fault, with no
	// trace of what the database was actually doing. Detached, a slow ping is
	// measured against our own budget: it either fails for a real reason worth
	// reporting, or it succeeds.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), healthPingTimeout)
	defer cancel()

	if s.db != nil {
		if err := s.db.Ping(ctx); err != nil {
			slog.ErrorContext(ctx, "health check db ping failed",
				"err", err, "handled", false, "timeout", healthPingTimeout.String())
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status": "unhealthy",
				"error":  "database unavailable",
				"time":   time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"time":    time.Now().UTC().Format(time.RFC3339),
		"version": s.version,
	})
}

func (s *Server) handleGetProjectHealth(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	ctx, span := tracing.StartSpan(r.Context(), "health.getProjectHealth",
		attribute.String("project.id", projectID),
	)
	defer span.End()

	health, err := s.projectHealth.GetProjectHealth(ctx, projectID)
	if err != nil {
		tracing.RecordError(span, err)
		slog.ErrorContext(ctx, "get project health", "project_id", projectID, "err", err)
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, health)
}

func (s *Server) handleResetProjectHealth(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	if err := s.projectHealth.ResetProjectHealth(r.Context(), projectID); err != nil {
		slog.ErrorContext(r.Context(), "reset project health", "project_id", projectID, "err", err)
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
