package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	// slugCacheTTL bounds how long a project id keeps its resolved slug.
	slugCacheTTL = 10 * time.Minute
	// maxTrackedHeaders caps the once-per-header warning memory. A sender can
	// invent header values without limit; when the set is full it starts over.
	maxTrackedHeaders = 1024
	// maxHeaderEcho caps how much of a client-supplied header the warning text
	// and the log line repeat.
	maxHeaderEcho = 64
)

type cachedSlug struct {
	slug    string
	expires time.Time
}

// projectNotes resolves project slugs for the 202 body and remembers which
// (project, header) pairs already produced a warning.
type projectNotes struct {
	mu     sync.Mutex
	slugs  map[string]cachedSlug
	warned map[string]struct{}
}

// projectSlug returns the slug of the project the API key is bound to, or ""
// when no lookup is configured or the project cannot be read. The answer is
// additive information, so a failed lookup never fails the request.
func (h *Handler) projectSlug(ctx context.Context, projectID string) string {
	if projectID == "" || h.ProjectSlug == nil {
		return ""
	}
	now := h.now()
	h.notes.mu.Lock()
	c, ok := h.notes.slugs[projectID]
	h.notes.mu.Unlock()
	if ok && now.Before(c.expires) {
		return c.slug
	}
	slug, err := h.ProjectSlug(ctx, projectID)
	if err != nil || slug == "" {
		return ""
	}
	h.notes.mu.Lock()
	if h.notes.slugs == nil {
		h.notes.slugs = map[string]cachedSlug{}
	}
	h.notes.slugs[projectID] = cachedSlug{slug: slug, expires: now.Add(slugCacheTTL)}
	h.notes.mu.Unlock()
	return slug
}

// CheckProjectHeader compares the x-funnelbarn-project header of a request
// whose API key is bound to a project with that project. It returns the text
// for the warning field of the 202 body when the header names another project
// (slug or id), and "" when the header is absent or matches. The first
// mismatch per project and header value is logged at Warn; Error would open a
// BugBarn issue for a client misconfiguration. The event stays with the key's
// project either way.
func (h *Handler) CheckProjectHeader(ctx context.Context, projectID, slug, header string) string {
	if header == "" || projectID == "" || header == slug || header == projectID {
		return ""
	}
	echo := header
	if len(echo) > maxHeaderEcho {
		echo = echo[:maxHeaderEcho]
	}
	h.notes.mu.Lock()
	if h.notes.warned == nil || len(h.notes.warned) >= maxTrackedHeaders {
		h.notes.warned = map[string]struct{}{}
	}
	k := projectID + "\x00" + echo
	_, seen := h.notes.warned[k]
	h.notes.warned[k] = struct{}{}
	h.notes.mu.Unlock()
	if !seen {
		slog.WarnContext(ctx, "ingest: x-funnelbarn-project names another project than the API key; the key wins",
			"project_id", projectID, "project", slug, "header", echo)
	}
	if slug == "" {
		return fmt.Sprintf("x-funnelbarn-project %q names another project; the event was filed under the project of the API key", echo)
	}
	return fmt.Sprintf("x-funnelbarn-project %q names another project; the event was filed under %q, the project of the API key", echo, slug)
}

// projectAnswer builds the additive fields of a 202 body: project (the slug
// the request was filed under, when known) and warning (when the project header
// names another project).
func (h *Handler) projectAnswer(ctx context.Context, projectID, header string) map[string]any {
	out := map[string]any{}
	slug := h.projectSlug(ctx, projectID)
	if slug != "" {
		out["project"] = slug
	}
	if w := h.CheckProjectHeader(ctx, projectID, slug, header); w != "" {
		out["warning"] = w
	}
	return out
}

// ProjectAnswer is the recordings handler's entry to the same fields: the key's
// project slug is already resolved there.
func (h *Handler) ProjectAnswer(ctx context.Context, projectID, slug, header string) map[string]any {
	out := map[string]any{"project": slug}
	if w := h.CheckProjectHeader(ctx, projectID, slug, header); w != "" {
		out["warning"] = w
	}
	return out
}
