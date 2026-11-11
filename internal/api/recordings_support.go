package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

const (
	recordingsNotConfigured       = "session recording not configured"
	recordingsNotConfiguredIngest = recordingsNotConfigured + " (R2 credentials missing)"
)

// recordingsGuard runs h when recording storage is configured and unavailable
// otherwise. Registered on the route, it runs where the handler's own nil check
// used to: after the session check on dashboard routes, before the key check on
// the API-key routes.
func (s *Server) recordingsGuard(h http.HandlerFunc, unavailable func(http.ResponseWriter)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.recordings == nil {
			unavailable(w)
			return
		}
		h(w, r)
	}
}

// withRecordings answers 503 when recording storage is not configured.
func (s *Server) withRecordings(h http.HandlerFunc) http.HandlerFunc {
	return s.recordingsGuard(h, func(w http.ResponseWriter) {
		jsonError(w, recordingsNotConfigured, http.StatusServiceUnavailable)
	})
}

// withRecordingsIngest is withRecordings for the chunk ingest route, whose
// message names the missing credentials.
func (s *Server) withRecordingsIngest(h http.HandlerFunc) http.HandlerFunc {
	return s.recordingsGuard(h, func(w http.ResponseWriter) {
		jsonError(w, recordingsNotConfiguredIngest, http.StatusServiceUnavailable)
	})
}

// The bodies the list routes answer with when recording storage is not
// configured. Read-only: writeJSON only encodes them.
var (
	emptyRecordingList = map[string]any{"recordings": []any{}, "limit": 50, "offset": 0}
	emptySessionIDs    = map[string]any{"session_ids": []any{}}
)

// withRecordingsOr answers 200 with an empty body when recording storage is
// not configured, for the list routes the dashboard calls on every project.
func (s *Server) withRecordingsOr(empty any, h http.HandlerFunc) http.HandlerFunc {
	return s.recordingsGuard(h, func(w http.ResponseWriter) { writeJSON(w, http.StatusOK, empty) })
}

// recordingPathIDs reads {id} and {rid}, answering 400 when either is empty.
func recordingPathIDs(w http.ResponseWriter, r *http.Request) (projectID, recordingID string, ok bool) {
	projectID, recordingID = r.PathValue("id"), r.PathValue("rid")
	if projectID == "" || recordingID == "" {
		jsonError(w, "project id and recording id are required", http.StatusBadRequest)
		return "", "", false
	}
	return projectID, recordingID, true
}

// chunkIndexParam reads {index}, answering 400 when it is not a non-negative integer.
func chunkIndexParam(w http.ResponseWriter, r *http.Request) (int, bool) {
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil || index < 0 {
		jsonError(w, "invalid chunk index", http.StatusBadRequest)
		return 0, false
	}
	return index, true
}

// startRecordingSpan opens a span carrying project.id, recording.id when there
// is one, and any extra attributes.
func startRecordingSpan(r *http.Request, name, projectID, recordingID string, extra ...attribute.KeyValue) (context.Context, trace.Span) {
	attrs := append([]attribute.KeyValue{attribute.String("project.id", projectID)}, extra...)
	if recordingID != "" {
		attrs = append(attrs, attribute.String("recording.id", recordingID))
	}
	return tracing.StartSpan(r.Context(), name, attrs...)
}

// failRecording answers a service error. A missing recording is an expected
// 404 and not a span error; anything else is recorded on the span first.
func failRecording(w http.ResponseWriter, span trace.Span, err error, op string) {
	if !domain.IsNotFound(err) {
		tracing.RecordError(span, err)
	}
	mapServiceError(w, err, op)
}

// serveRecordingChunk writes one recording chunk. A missing chunk is a 404; any
// other storage failure is logged as failMsg and answers 500.
func (s *Server) serveRecordingChunk(w http.ResponseWriter, ctx context.Context, span trace.Span, projectID, recordingID string, index int, failMsg string) {
	data, err := s.recordings.GetChunk(ctx, projectID, recordingID, index)
	if err != nil {
		if isChunkNotFound(err) {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}
		tracing.RecordError(span, err)
		slog.ErrorContext(ctx, failMsg,
			"error", err.Error(), "handled", false,
			"recording_id", recordingID,
			"project_id", projectID,
			"chunk_index", index,
			"request_id", RequestIDFromContext(ctx),
		)
		jsonError(w, "failed to fetch chunk", http.StatusInternalServerError)
		return
	}
	span.SetAttributes(attribute.Int("chunk.bytes", len(data)))

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.WriteHeader(http.StatusOK)
	w.Write(data) //nolint:errcheck
}
