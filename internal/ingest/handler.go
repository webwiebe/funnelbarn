package ingest

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

const defaultQueueSize = 32768

// Handler receives ingest HTTP requests, validates them, and enqueues to the spool.
type Handler struct {
	auth         *auth.Authorizer
	spool        *spool.Spool
	maxBodyBytes int64
	now          func() time.Time
	idFn         func() string
	queue        chan spool.Record
}

// NewHandler creates an ingest Handler.
func NewHandler(authorizer *auth.Authorizer, eventSpool *spool.Spool, maxBodyBytes int64) *Handler {
	if maxBodyBytes <= 0 {
		maxBodyBytes = 1 << 20
	}
	return &Handler{
		auth:         authorizer,
		spool:        eventSpool,
		maxBodyBytes: maxBodyBytes,
		now:          time.Now,
		idFn:         generateIngestID,
		queue:        make(chan spool.Record, defaultQueueSize),
	}
}

// Start drains the in-memory queue and flushes batches to the spool file.
// Returns when ctx is cancelled, flushing remaining records first.
func (h *Handler) Start(ctx context.Context) {
	const maxBatch = 64
	batch := make([]spool.Record, 0, maxBatch)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := h.spool.AppendBatch(batch); err != nil {
			if !errors.Is(err, spool.ErrFull) {
				slog.Error("ingest spool batch write", "err", err)
			}
		}
		batch = batch[:0]
	}

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case r := <-h.queue:
			batch = append(batch, r)
			if len(batch) >= maxBatch {
				flush()
			}
		case <-ticker.C:
			flush()
			metrics.IngestQueueDepth.Set(float64(len(h.queue)))
		case <-ctx.Done():
			// Drain whatever is left before exiting.
			for {
				select {
				case r := <-h.queue:
					batch = append(batch, r)
				default:
					flush()
					return
				}
			}
		}
	}
}

// ValidAPIKey validates the API key on the request.
func (h *Handler) ValidAPIKey(r *http.Request) bool {
	_, _, ok := h.APIKeyProjectScope(r)
	return ok
}

// APIKeyProjectScope validates the API key and returns project ID, scope, and ok.
func (h *Handler) APIKeyProjectScope(r *http.Request) (projectID string, scope string, ok bool) {
	if h == nil || h.auth == nil {
		return "", "full", true
	}
	return h.auth.ValidWithProject(r.Context(), r.Header.Get(auth.HeaderAPIKey))
}

// ServeHTTP handles POST /api/v1/events.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.auth == nil || h.spool == nil {
		// Service mis-configuration: a 503 here means the process is up but
		// ingest is not wired correctly. Surface to BugBarn.
		slog.ErrorContext(r.Context(), "ingest unavailable: handler not initialised",
			"handled", false,
			"has_auth", h != nil && h.auth != nil,
			"has_spool", h != nil && h.spool != nil,
		)
		jsonErr(w, "ingest unavailable", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodPost:
	default:
		w.Header().Set("Allow", http.MethodPost)
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID, _, ok := h.APIKeyProjectScope(r)
	if !ok {
		// Routine condition (rotated keys, misconfigured clients) — Warn so
		// a sudden spike is visible in BugBarn without flooding it.
		slog.WarnContext(r.Context(), "ingest: unauthorized",
			"user_agent", r.Header.Get("User-Agent"),
		)
		jsonErr(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	defer r.Body.Close()

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.maxBodyBytes))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			// 413s used to be silent on the recording-chunk endpoint and
			// caused weeks of lost data. Surface them everywhere.
			slog.ErrorContext(r.Context(), "ingest body too large",
				"err", err, "handled", false,
				"project_id", projectID,
				"content_length", r.ContentLength,
				"limit_bytes", h.maxBodyBytes,
				"user_agent", r.Header.Get("User-Agent"),
			)
			jsonErr(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		slog.WarnContext(r.Context(), "ingest: unable to read body",
			"err", err, "handled", true,
			"project_id", projectID,
		)
		jsonErr(w, "unable to read request body", http.StatusBadRequest)
		return
	}

	ingestID := h.idFn()

	_, span := tracing.StartSpan(r.Context(), "ingest.enqueue",
		attribute.String("ingest.id", ingestID),
		attribute.String("project.id", projectID),
		attribute.Int64("body.size", int64(len(body))),
	)
	defer span.End()

	projectSlug := r.Header.Get("x-funnelbarn-project")
	if projectSlug == "" {
		projectSlug = projectID
	}

	record := spool.Record{
		IngestID:      ingestID,
		ReceivedAt:    h.now().UTC(),
		ContentType:   r.Header.Get("Content-Type"),
		RemoteAddr:    r.RemoteAddr,
		ClientIP:      extractClientIP(r),
		ContentLength: int64(len(body)),
		BodyBase64:    base64.StdEncoding.EncodeToString(body),
		ProjectSlug:   projectSlug,
	}

	select {
	case h.queue <- record:
		metrics.EventsIngested.Inc()
	default:
		// Queue saturation is a real operational signal — events get dropped
		// silently on the client side after a 429, so make it visible.
		queueErr := errors.New("ingest in-memory queue is full")
		tracing.RecordError(span, queueErr)
		slog.ErrorContext(r.Context(), "ingest queue full, dropping event",
			"err", queueErr, "handled", false,
			"project_id", projectID,
			"ingest_id", ingestID,
			"queue_cap", cap(h.queue),
		)
		w.Header().Set("Retry-After", "1")
		jsonErr(w, "ingest queue full", http.StatusTooManyRequests)
		return
	}

	slog.Info("event enqueued", "ingest_id", ingestID, "project", projectSlug)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"accepted": true,
		"ingestId": ingestID,
	})
}

func jsonErr(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// extractClientIP returns the best-guess real client IP, preferring CDN headers
// over X-Forwarded-For and falling back to the TCP remote address. Safe for
// analytics use (not security-critical — geo data from a spoofed IP only
// affects the spoofer's own session).
func extractClientIP(r *http.Request) string {
	if v := r.Header.Get("CF-Connecting-IP"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if idx := strings.IndexByte(v, ','); idx >= 0 {
			v = strings.TrimSpace(v[:idx])
		} else {
			v = strings.TrimSpace(v)
		}
		if v != "" {
			return v
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func generateIngestID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		// crypto/rand failing is a system-level problem (no entropy source).
		// Log it so we know — the timestamp fallback keeps ingest working.
		slog.Error("crypto/rand failed; using timestamp fallback for ingest id",
			"err", err, "handled", true)
		return time.Now().UTC().Format("20060102T150405.000000000Z") + "-fallback"
	}
	return hex.EncodeToString(raw[:])
}
