package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// maxRecordingChunkBytes caps the size of a single recording-chunk POST body.
// The first chunk of any rrweb recording contains a full DOM snapshot, which is
// commonly 1-5 MiB and can exceed that on content-heavy pages. The previous
// default cap of 256 KiB silently truncated the snapshot, causing the server to
// reject chunk 0 with a JSON parse error and leaving every recording with
// first_chunk_index >= 1 (snapshot lost forever).
const maxRecordingChunkBytes = 10 << 20 // 10 MiB

func (s *Server) handleIngestRecordingChunk(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		jsonError(w, "session recording not configured (R2 credentials missing)", http.StatusServiceUnavailable)
		return
	}

	projectID, _, ok := s.ingest.APIKeyProjectScope(r)
	if !ok {
		// Warn rather than Error: invalid API keys happen routinely (rotated
		// keys, misconfigured clients) but a sudden surge points at trouble.
		slog.WarnContext(r.Context(), "recording chunk: unauthorized",
			"user_agent", r.Header.Get("User-Agent"),
			"request_id", RequestIDFromContext(r.Context()),
		)
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if s.projectHealth != nil {
		pid := projectID
		go func() {
			if err := s.projectHealth.MarkRecordingsReceived(context.Background(), pid); err != nil {
				slog.Warn("recording chunk: mark health", "project_id", pid, "err", err)
			}
		}()
	}

	proj, err := s.projects.GetProject(r.Context(), projectID)
	if err != nil {
		mapServiceError(w, err, "handleIngestRecordingChunk")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRecordingChunkBytes)
	var chunk service.RecordingChunk
	if err := readJSON(r, &chunk); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			// Surface 413s to BugBarn — this used to be the silent failure
			// mode where the SDK retried/abandoned and we never knew.
			slog.ErrorContext(r.Context(), "recording chunk rejected: body too large",
				"err", err, "handled", false,
				"project_id", projectID,
				"content_length", r.ContentLength,
				"limit_bytes", int64(maxRecordingChunkBytes),
				"user_agent", r.Header.Get("User-Agent"),
				"request_id", RequestIDFromContext(r.Context()),
			)
			jsonError(w, "recording chunk too large", http.StatusRequestEntityTooLarge)
			return
		}
		// JSON parse errors on this endpoint are unusual (the SDK is the only
		// producer); log them so we can spot SDK regressions.
		slog.WarnContext(r.Context(), "recording chunk rejected: invalid json",
			"err", err, "handled", true,
			"project_id", projectID,
			"content_length", r.ContentLength,
			"user_agent", r.Header.Get("User-Agent"),
			"request_id", RequestIDFromContext(r.Context()),
		)
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if chunk.RecordingID == "" || chunk.SessionID == "" || len(chunk.Events) == 0 {
		jsonError(w, "recording_id, session_id, and events are required", http.StatusUnprocessableEntity)
		return
	}

	chunk.ProjectID = projectID
	chunk.ProjectSlug = proj.Slug
	chunk.Environment = r.Header.Get("x-funnelbarn-environment")
	chunk.UserAgent = r.Header.Get("User-Agent")

	if err := s.recordings.IngestChunk(r.Context(), chunk); err != nil {
		slog.ErrorContext(r.Context(), "ingest recording chunk failed",
			"error", err, "handled", false,
			"recording_id", chunk.RecordingID,
			"project_id", chunk.ProjectID,
			"chunk_index", chunk.ChunkIndex,
			"request_id", RequestIDFromContext(r.Context()),
		)
		jsonError(w, "failed to ingest chunk", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
}

func (s *Server) handleListRecordings(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		writeJSON(w, http.StatusOK, map[string]any{"recordings": []any{}, "limit": 50, "offset": 0})
		return
	}
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	opts := repository.RecordingListOpts{
		Limit:       limit,
		Offset:      offset,
		Environment: r.URL.Query().Get("environment"),
		DeviceType:  r.URL.Query().Get("device_type"),
		PageURL:     r.URL.Query().Get("page_url"),
		HumanOnly:   r.URL.Query().Get("human_only") == "true",
	}
	if ids := r.URL.Query().Get("session_ids"); ids != "" {
		for _, id := range strings.Split(ids, ",") {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				opts.SessionIDs = append(opts.SessionIDs, trimmed)
			}
		}
	}

	recordings, err := s.recordings.ListRecordings(r.Context(), projectID, opts)
	if err != nil {
		mapServiceError(w, err, "handleListRecordings")
		return
	}
	if recordings == nil {
		recordings = []repository.Recording{}
	}
	addPaginationHeaders(w, r, limit, offset, len(recordings))
	writeJSON(w, http.StatusOK, map[string]any{
		"recordings": recordings,
		"limit":      limit,
		"offset":     offset,
	})
}

func (s *Server) handleGetRecordingChunk(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		jsonError(w, "session recording not configured", http.StatusServiceUnavailable)
		return
	}
	projectID := r.PathValue("id")
	recordingID := r.PathValue("rid")
	indexStr := r.PathValue("index")
	if projectID == "" || recordingID == "" || indexStr == "" {
		jsonError(w, "project id, recording id, and chunk index are required", http.StatusBadRequest)
		return
	}
	index, err := strconv.Atoi(indexStr)
	if err != nil || index < 0 {
		jsonError(w, "invalid chunk index", http.StatusBadRequest)
		return
	}

	data, err := s.recordings.GetChunk(r.Context(), projectID, recordingID, index)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "not found") || strings.Contains(errStr, "NoSuchKey") || strings.Contains(errStr, "404") {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}
		slog.ErrorContext(r.Context(), "fetch recording chunk failed",
			"error", err, "handled", false,
			"recording_id", recordingID,
			"project_id", projectID,
			"chunk_index", index,
			"request_id", RequestIDFromContext(r.Context()),
		)
		jsonError(w, "failed to fetch chunk", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.WriteHeader(http.StatusOK)
	w.Write(data) //nolint:errcheck
}

func (s *Server) handleDeleteRecording(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		jsonError(w, "session recording not configured", http.StatusServiceUnavailable)
		return
	}
	projectID := r.PathValue("id")
	recordingID := r.PathValue("rid")
	if projectID == "" || recordingID == "" {
		jsonError(w, "project id and recording id are required", http.StatusBadRequest)
		return
	}

	if err := s.recordings.DeleteRecording(r.Context(), projectID, recordingID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}
		slog.ErrorContext(r.Context(), "delete recording failed",
			"error", err, "handled", false,
			"recording_id", recordingID,
			"project_id", projectID,
			"request_id", RequestIDFromContext(r.Context()),
		)
		jsonError(w, "failed to delete recording", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetRecordingFlags(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		jsonError(w, "session recording not configured", http.StatusServiceUnavailable)
		return
	}
	projectID := r.PathValue("id")
	recordingID := r.PathValue("rid")
	if projectID == "" || recordingID == "" {
		jsonError(w, "project id and recording id are required", http.StatusBadRequest)
		return
	}

	sessionID, err := s.recordings.GetRecordingSessionID(r.Context(), projectID, recordingID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}
		mapServiceError(w, err, "handleGetRecordingFlags")
		return
	}

	evals, err := s.recordings.FlagEvaluationsForSession(r.Context(), projectID, sessionID)
	if err != nil {
		mapServiceError(w, err, "handleGetRecordingFlags")
		return
	}
	if evals == nil {
		evals = []repository.FlagEvaluationEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"evaluations": evals})
}

// handleLookupTrace resolves a W3C trace_id (as seen in SpanBarn/BugBarn) to the
// FunnelBarn session + recording that captured it, plus the seek offset. This is
// the cross-stack deep-link: given an error trace, find the replayable session.
// Auth is by API key (project scope) so a trace only resolves within its project.
func (s *Server) handleLookupTrace(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		jsonError(w, "session recording not configured", http.StatusServiceUnavailable)
		return
	}
	projectID, _, ok := s.ingest.APIKeyProjectScope(r)
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	traceID := r.PathValue("trace_id")
	if traceID == "" {
		jsonError(w, "trace_id required", http.StatusBadRequest)
		return
	}

	lookup, found, err := s.recordings.LookupTrace(r.Context(), projectID, traceID)
	if err != nil {
		mapServiceError(w, err, "handleLookupTrace")
		return
	}
	if !found {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, lookup)
}

// handleGetRecordingTraces returns the ordered trace timeline for a recording so
// the replay UI can overlay trace markers on the scrubber.
func (s *Server) handleGetRecordingTraces(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		jsonError(w, "session recording not configured", http.StatusServiceUnavailable)
		return
	}
	projectID := r.PathValue("id")
	recordingID := r.PathValue("rid")
	if projectID == "" || recordingID == "" {
		jsonError(w, "project id and recording id are required", http.StatusBadRequest)
		return
	}

	traces, err := s.recordings.TracesForRecording(r.Context(), projectID, recordingID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}
		mapServiceError(w, err, "handleGetRecordingTraces")
		return
	}
	if traces == nil {
		traces = []repository.TraceLink{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"traces": traces})
}

func (s *Server) handleFunnelStepSessions(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		writeJSON(w, http.StatusOK, map[string]any{"session_ids": []any{}})
		return
	}
	projectID := r.PathValue("id")
	funnelID := r.PathValue("fid")
	stepStr := r.PathValue("step")
	if projectID == "" || funnelID == "" || stepStr == "" {
		jsonError(w, "project id, funnel id, and step required", http.StatusBadRequest)
		return
	}
	step, err := strconv.Atoi(stepStr)
	if err != nil || step < 1 {
		jsonError(w, "invalid step number", http.StatusBadRequest)
		return
	}

	from := time.Now().AddDate(0, 0, -30)
	to := time.Now()
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}

	sessionIDs, err := s.recordings.SessionsAtStep(r.Context(), funnelID, projectID, step, from, to)
	if err != nil {
		mapServiceError(w, err, "handleFunnelStepSessions")
		return
	}
	if sessionIDs == nil {
		sessionIDs = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"session_ids": sessionIDs})
}

func (s *Server) handleFlowPageSessions(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		writeJSON(w, http.StatusOK, map[string]any{"session_ids": []any{}})
		return
	}
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	page := r.URL.Query().Get("page")
	if page == "" {
		jsonError(w, "page query parameter required", http.StatusBadRequest)
		return
	}

	from := time.Now().AddDate(0, 0, -30)
	to := time.Now()
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}

	sessionIDs, err := s.recordings.SessionsForPage(r.Context(), projectID, page, from, to)
	if err != nil {
		mapServiceError(w, err, "handleFlowPageSessions")
		return
	}
	if sessionIDs == nil {
		sessionIDs = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"session_ids": sessionIDs})
}
