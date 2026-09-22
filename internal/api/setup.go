package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/bblog"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// setupKey derives a deterministic ingest API key from the session secret and
// slug. Returns ok=false when no session secret is configured — in that case we
// MUST NOT derive a key from a hardcoded constant, which would make every
// project's ingest key globally predictable. Callers fail closed instead.
func setupKey(secret, slug string) (plaintext, keySHA256 string, ok bool) {
	if strings.TrimSpace(secret) == "" {
		return "", "", false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("setup:" + slug))
	raw := mac.Sum(nil)
	plaintext = hex.EncodeToString(raw)[:40]
	sum := sha256.Sum256([]byte(plaintext))
	keySHA256 = hex.EncodeToString(sum[:])
	return plaintext, keySHA256, true
}

// errSetupKeyUnavailable is returned when no session secret is configured, so
// the setup guide's ingest key cannot be derived. Deriving one from a
// hardcoded constant would make every project's ingest key globally
// predictable, so callers fail closed instead.
var errSetupKeyUnavailable = errors.New("setup unavailable: server missing session secret")

// handleSetup is a public endpoint that returns a self-service setup page in
// Markdown format. It creates the project (status='pending') if it doesn't
// exist yet and upserts a deterministic ingest API key.
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
		if errors.Is(err, errSetupKeyUnavailable) {
			slog.Error("setup: refusing to derive key without a session secret", "slug", slug, "handled", false)
			http.Error(w, "setup unavailable: server missing session secret", http.StatusServiceUnavailable)
			return
		}
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
// and performs the side effects the document depends on: deriving (and
// upserting) the project's deterministic ingest key, and marking the
// project's health as "setup called".
//
// Doing that mark here, not only from handleSetup, is a deliberate choice.
// get_setup_guide over MCP returns the identical document a visitor gets from
// GET /api/v1/setup/{slug}, so an assistant fetching it counts as the same
// "setup called" event this codebase already tracks for that endpoint. This
// does mean an assistant re-reading the guide, rather than an SDK actually
// calling home, flips the health flag. That's accepted: a false "setup
// called" only silences a health nag, while a false negative would leave a
// real integration looking unconfigured.
func (s *Server) renderSetupDoc(ctx context.Context, project repository.Project, publicURL string) (string, error) {
	plaintext, keySHA256, ok := setupKey(s.sessionSecret, project.Slug)
	if !ok {
		return "", errSetupKeyUnavailable
	}

	if err := s.projects.EnsureSetupAPIKey(ctx, project.ID, keySHA256); err != nil {
		return "", err
	}

	if s.projectHealth != nil {
		pid := project.ID
		bblog.Go("setup-health", func() {
			if err := s.projectHealth.MarkSetupCalled(context.Background(), pid); err != nil {
				slog.Warn("setup: mark health", "project_id", pid, "err", err)
			}
		})
	}

	return buildSetupMarkdown(project, plaintext, publicURL, s.flagAutoRegisterMax, s.mcpEnabled()), nil
}
