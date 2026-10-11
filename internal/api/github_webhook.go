package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/ghwebhook"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

// githubWebhookPath receives GitHub webhook deliveries for the agents
// delivery funnels (#331).
const githubWebhookPath = "/api/v1/integrations/github"

// githubWebhookMaxBody caps a delivery. It matches the default body cap every
// other non-ingest route gets in ServeHTTP.
const githubWebhookMaxBody = 256 << 10

// githubWebhook verifies a delivery's signature, maps it to at most one
// event and enqueues it on the ingest path, keyed on X-GitHub-Delivery so a
// redelivery is stored once.
type githubWebhook struct {
	secret   string
	project  string
	ingest   *ingest.Handler
	clientIP func(*http.Request) string
	now      func() time.Time
}

// SetGitHubWebhook serves POST /api/v1/integrations/github with secret, filing
// events into the project with slug project. With an empty secret the route
// is not registered and answers 404.
func (s *Server) SetGitHubWebhook(secret, project string) {
	if secret == "" || s.ingest == nil {
		return
	}
	h := &githubWebhook{secret: secret, project: project, ingest: s.ingest, clientIP: s.clientIP, now: time.Now}
	s.mux.Handle("POST "+githubWebhookPath, s.limit(s.eventsLimiter, h))
}

func (h *githubWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, githubWebhookMaxBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			// GitHub does not redeliver on its own, so this event is lost
			// unless someone redelivers it by hand.
			slog.ErrorContext(r.Context(), "github webhook: delivery too large",
				"err", err, "handled", false,
				"delivery", r.Header.Get("X-GitHub-Delivery"),
				"event", r.Header.Get("X-GitHub-Event"),
				"limit_bytes", githubWebhookMaxBody)
			jsonError(w, "delivery too large", http.StatusRequestEntityTooLarge)
			return
		}
		jsonError(w, "unable to read request body", http.StatusBadRequest)
		return
	}
	if !ghwebhook.ValidSignature(h.secret, r.Header.Get("X-Hub-Signature-256"), body) {
		slog.WarnContext(r.Context(), "github webhook: bad or missing signature",
			"delivery", r.Header.Get("X-GitHub-Delivery"))
		jsonError(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	delivery := r.Header.Get("X-GitHub-Delivery")
	kind := r.Header.Get("X-GitHub-Event")
	if delivery == "" || kind == "" {
		jsonError(w, "X-GitHub-Delivery and X-GitHub-Event are required", http.StatusBadRequest)
		return
	}

	ev, ok, err := ghwebhook.Map(kind, body)
	if err != nil {
		slog.WarnContext(r.Context(), "github webhook: undecodable delivery",
			"err", err, "delivery", delivery, "event", kind)
		jsonError(w, "invalid payload", http.StatusBadRequest)
		return
	}
	if !ok {
		writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "stored": false})
		return
	}

	payload, err := json.Marshal(ev)
	if err != nil {
		jsonError(w, "encode event", http.StatusInternalServerError)
		return
	}
	record := spool.Record{
		IngestID:      delivery,
		ReceivedAt:    h.now().UTC(),
		ContentType:   "application/json",
		RemoteAddr:    r.RemoteAddr,
		ClientIP:      h.clientIP(r),
		ContentLength: int64(len(payload)),
		BodyBase64:    base64.StdEncoding.EncodeToString(payload),
		ProjectSlug:   h.project,
		UserAgent:     r.Header.Get("User-Agent"),
	}
	if err := h.ingest.Enqueue(record); err != nil {
		slog.ErrorContext(r.Context(), "github webhook: ingest queue full, dropping delivery",
			"err", err, "handled", false, "delivery", delivery, "event", ev.Name)
		w.Header().Set("Retry-After", "1")
		jsonError(w, "ingest queue full", http.StatusTooManyRequests)
		return
	}
	slog.InfoContext(r.Context(), "github webhook: event enqueued",
		"delivery", delivery, "event", ev.Name, "issue_ref", ev.UserID)
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "stored": true, "event": ev.Name, "ingestId": delivery})
}
