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
	"net/http"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
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
		http.Error(w, "ingest unavailable", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodPost:
	default:
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID, _, ok := h.APIKeyProjectScope(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	defer r.Body.Close()

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.maxBodyBytes))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "unable to read request body", http.StatusBadRequest)
		return
	}

	ingestID := h.idFn()

	// Derive project slug from the API key's project ID or the header.
	projectSlug := r.Header.Get("x-funnelbarn-project")
	if projectSlug == "" {
		projectSlug = projectID
	}

	record := spool.Record{
		IngestID:      ingestID,
		ReceivedAt:    h.now().UTC(),
		ContentType:   r.Header.Get("Content-Type"),
		RemoteAddr:    r.RemoteAddr,
		ContentLength: int64(len(body)),
		BodyBase64:    base64.StdEncoding.EncodeToString(body),
		ProjectSlug:   projectSlug,
	}

	select {
	case h.queue <- record:
		metrics.EventsIngested.Inc()
	default:
		w.Header().Set("Retry-After", "1")
		http.Error(w, "ingest queue full", http.StatusTooManyRequests)
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

func generateIngestID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return time.Now().UTC().Format("20060102T150405.000000000Z") + "-fallback"
	}
	return hex.EncodeToString(raw[:])
}
