package api

import (
	"context"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/bblog"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// setupKeyPlaceholder stands in for the ingest key wherever the setup guide
// shows one. The guide used to print a key derived from the session secret
// and the slug, and re-created it on every fetch, so a revoked setup key came
// back the next time anyone opened the guide. Keys now come only from
// POST /api/v1/apikeys (Settings → API keys), which shows each one once.
const setupKeyPlaceholder = "<your-ingest-api-key>"

// handleSetup returns the setup guide in Markdown format. It needs a dashboard
// session (see registerRoutes): an anonymous visitor used to get a working
// ingest key for any slug. It creates the project (status='pending') if it
// doesn't exist yet.
//
// GET /api/v1/setup/{slug}
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		http.Error(w, "slug is required", http.StatusBadRequest)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "setup.provision",
		attribute.String("project.slug", slug),
	)
	defer span.End()

	// Ensure project exists (pending if new).
	project, err := s.projects.EnsureProjectPending(ctx, slug, slug)
	if err != nil {
		tracing.RecordError(span, err)
		slog.Error("setup: ensure project", "slug", slug, "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	span.SetAttributes(
		attribute.String("project.id", project.ID),
		attribute.String("project.status", project.Status),
	)

	doc, err := s.renderSetupDoc(ctx, project, s.setupBaseURL(r))
	if err != nil {
		tracing.RecordError(span, err)
		slog.Error("setup: render doc", "project_id", project.ID, "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(doc))
}

// SetupDoc renders the same markdown document as GET /api/v1/setup/{slug},
// for an MCP tool call that has no *http.Request to read a branded-alias host
// from. It falls back to the same default setupBaseURL uses for a
// non-branded request: the configured FUNNELBARN_PUBLIC_URL, or the canonical
// funnelbarn.wiebe.xyz. Its signature matches mcp.Deps.SetupDoc exactly, so
// registerMCPRoutes wires it in with no wrapper.
//
// Unlike handleSetup, it does not create the project. An MCP tool call
// resolves an existing project (see mcp.Call.Project). Reading a setup guide
// should not conjure a pending project into existence.
func (s *Server) SetupDoc(ctx context.Context, project repository.Project) (string, error) {
	return s.renderSetupDoc(ctx, project, s.defaultPublicURL())
}

// renderSetupDoc renders the setup guide for project, addressed at publicURL,
// and marks the project's health as "setup called". It creates no API key:
// the guide shows setupKeyPlaceholder and tells the reader where to create one.
//
// Doing that mark here, not only from handleSetup, is a deliberate choice.
// get_setup_guide over MCP returns the identical document a visitor gets from
// GET /api/v1/setup/{slug}, so an assistant fetching it counts as the same
// "setup called" event this codebase already tracks for that endpoint. This
// does mean an assistant re-reading the guide, rather than an SDK actually
// calling home, flips the health flag. That's accepted: a false "setup
// called" only silences a health nag, while a false negative would leave a
// real integration looking unconfigured.
func (s *Server) renderSetupDoc(_ context.Context, project repository.Project, publicURL string) (string, error) {
	if s.projectHealth != nil {
		pid := project.ID
		bblog.Go("setup-health", func() {
			if err := s.projectHealth.MarkSetupCalled(context.Background(), pid); err != nil {
				slog.Warn("setup: mark health", "project_id", pid, "err", err)
			}
		})
	}

	return buildSetupMarkdown(project, setupKeyPlaceholder, publicURL, s.flagAutoRegisterMax, s.mcpEnabled()), nil
}
