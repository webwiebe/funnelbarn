package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// maxRecordingChunkBytes caps the size of a single recording-chunk POST body.
// The first chunk of any rrweb recording contains a full DOM snapshot, which is
// commonly 1-5 MiB and can exceed that on content-heavy pages. The previous
// default cap of 256 KiB silently truncated the snapshot, causing the server to
// reject chunk 0 with a JSON parse error and leaving every recording with
// first_chunk_index >= 1 (snapshot lost forever).
const maxRecordingChunkBytes = 10 << 20 // 10 MiB

func (s *Server) handleIngestRecordingChunk(w http.ResponseWriter, r *http.Request) {
	projectID, _, ok := s.ingest.APIKeyProjectScope(r)
	if !ok {
		// Warn, not Error: a caller presenting a key we do not accept is not a
		// fault in this service. It happens routinely — rotated keys,
		// misconfigured clients, and crawlers poking the endpoint. bblog
		// forwards Error and above to BugBarn, so this stays out of the issue
		// list; a surge shows up as the 401 rate on this path in
		// funnelbarn_http_requests_total, which is where a rate belongs.
		slog.WarnContext(r.Context(), "recording chunk: unauthorized",
			"user_agent", r.Header.Get("User-Agent"),
			"request_id", RequestIDFromContext(r.Context()),
		)
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, span := tracing.StartSpan(r.Context(), "recordings.ingest_chunk",
		attribute.String("project.id", projectID),
	)
	defer span.End()

	// The global env-var key (FUNNELBARN_API_KEY) resolves with an empty
	// project ID. Recording chunks must be attributed to a specific project, so
	// this key cannot be used here. A common misconfiguration is setting
	// BUGBARN_FUNNELBARN_API_KEY (or equivalent cross-barn env vars) to the
	// global key instead of a per-project key from the project settings page.
	if projectID == "" {
		slog.WarnContext(ctx, "recording chunk: global API key cannot be used for recording; project-scoped key required",
			"user_agent", r.Header.Get("User-Agent"),
			"request_id", RequestIDFromContext(ctx),
		)
		jsonError(w, "recording requires a project-scoped API key — use the key from the project settings page, not the global FUNNELBARN_API_KEY", http.StatusUnauthorized)
		return
	}

	proj, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		// A key that resolves to a project that no longer exists is a client
		// misconfiguration (404), not a server fault — log it so the case is
		// distinguishable, but keep it off the span's error count.
		if domain.IsNotFound(err) {
			slog.WarnContext(ctx, "recording chunk: project not found for API key",
				"project_id", projectID,
				"user_agent", r.Header.Get("User-Agent"),
				"request_id", RequestIDFromContext(ctx),
			)
		} else {
			tracing.RecordError(span, err)
		}
		mapServiceError(w, err, "handleIngestRecordingChunk")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRecordingChunkBytes)
	chunk, ok := readRecordingChunk(ctx, span, w, r, projectID)
	if !ok {
		return
	}
	if !validRecordingChunk(w, chunk) {
		return
	}

	chunk.ProjectID = projectID
	chunk.ProjectSlug = proj.Slug
	chunk.Environment = r.Header.Get("x-funnelbarn-environment")
	chunk.UserAgent = r.Header.Get("User-Agent")

	span.SetAttributes(
		attribute.String("recording.id", chunk.RecordingID),
		attribute.String("session.id", chunk.SessionID),
		attribute.Int("chunk.index", chunk.ChunkIndex),
		attribute.Int("chunk.events.count", len(chunk.Events)),
	)

	if err := s.ingestChunkDetached(ctx, chunk); err != nil {
		tracing.RecordError(span, err)
		slog.ErrorContext(ctx, "ingest recording chunk failed",
			"error", err.Error(), "handled", false,
			"recording_id", chunk.RecordingID,
			"project_id", chunk.ProjectID,
			"chunk_index", chunk.ChunkIndex,
			"request_id", RequestIDFromContext(ctx),
		)
		jsonError(w, "failed to ingest chunk", http.StatusInternalServerError)
		return
	}
	// Only a stored chunk counts: marking before validation would show a
	// project whose chunks are all rejected as receiving recordings.
	s.markRecordingsReceived(projectID)
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
}

func readRecordingChunk(ctx context.Context, span trace.Span, w http.ResponseWriter, r *http.Request, projectID string) (service.RecordingChunk, bool) {
	var chunk service.RecordingChunk
	if err := readJSON(r, &chunk); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			// Surface 413s to BugBarn — this used to be the silent failure
			// mode where the SDK retried/abandoned and we never knew.
			tracing.RecordError(span, err)
			slog.ErrorContext(ctx, "recording chunk rejected: body too large",
				"err", err, "handled", false,
				"project_id", projectID,
				"content_length", r.ContentLength,
				"limit_bytes", int64(maxRecordingChunkBytes),
				"user_agent", r.Header.Get("User-Agent"),
				"request_id", RequestIDFromContext(ctx),
			)
			jsonError(w, "recording chunk too large", http.StatusRequestEntityTooLarge)
			return service.RecordingChunk{}, false
		}
		// JSON parse errors on this endpoint are unusual (the SDK is the only
		// producer); log them so we can spot SDK regressions.
		slog.WarnContext(ctx, "recording chunk rejected: invalid json",
			"err", err, "handled", true,
			"project_id", projectID,
			"content_length", r.ContentLength,
			"user_agent", r.Header.Get("User-Agent"),
			"request_id", RequestIDFromContext(ctx),
		)
		jsonError(w, "invalid json", http.StatusBadRequest)
		return service.RecordingChunk{}, false
	}
	return chunk, true
}

// validRecordingChunk writes the 422 and returns false when the chunk lacks
// required fields or carries an id that is unsafe as an object-storage key.
func validRecordingChunk(w http.ResponseWriter, chunk service.RecordingChunk) bool {
	if chunk.RecordingID == "" || chunk.SessionID == "" || len(chunk.Events) == 0 {
		jsonError(w, "recording_id, session_id, and events are required", http.StatusUnprocessableEntity)
		return false
	}
	// recording_id and session_id are interpolated into R2 object keys
	// (recordings/<project>/<recording_id>/<index>). Reject anything that could
	// inject or traverse a key path ('/', '..', control chars) before it reaches
	// storage — see chunkKey in internal/service/recordings.go.
	if !isValidStorageID(chunk.RecordingID) || !isValidStorageID(chunk.SessionID) {
		jsonError(w, "recording_id and session_id must be alphanumeric (with '-' or '_'), max 64 chars", http.StatusUnprocessableEntity)
		return false
	}
	return true
}

// isValidStorageID guards IDs that become object-storage key segments. It
// rejects anything outside [A-Za-z0-9_-] (notably '/', '.', and control chars)
// so a client-supplied recording_id/session_id cannot inject or traverse R2
// keys. 64 chars is comfortably above the SDK's 32-hex-char IDs.
func isValidStorageID(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if allowed := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_'; !allowed {
			return false
		}
	}
	return true
}

// parseRecordingListOpts reads the recordings list filters. limit defaults to
// 50 and offset to 0; a value that is not a number, or out of range (limit 1 to
// 500, offset from 0), keeps the default. session_ids is a comma-separated list.
func parseRecordingListOpts(q url.Values) repository.RecordingListOpts {
	opts := repository.RecordingListOpts{
		Limit:       50,
		Environment: q.Get("environment"),
		DeviceType:  q.Get("device_type"),
		PageURL:     q.Get("page_url"),
		HumanOnly:   q.Get("human_only") == "true",
	}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 && n <= 500 {
		opts.Limit = n
	}
	if n, err := strconv.Atoi(q.Get("offset")); err == nil && n >= 0 {
		opts.Offset = n
	}
	if ids := splitCSV(q.Get("session_ids")); len(ids) > 0 {
		opts.SessionIDs = ids
	}
	return opts
}

func (s *Server) handleListRecordings(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}

	opts := parseRecordingListOpts(r.URL.Query())
	limit, offset := opts.Limit, opts.Offset

	ctx, span := startRecordingSpan(r, "recordings.list", projectID, "",
		attribute.Int("limit", limit),
		attribute.Int("offset", offset),
		attribute.Bool("human_only", opts.HumanOnly),
	)
	defer span.End()

	recordings, err := s.recordings.ListRecordings(ctx, projectID, opts)
	if err != nil {
		failRecording(w, span, err, "handleListRecordings")
		return
	}
	if recordings == nil {
		recordings = []repository.Recording{}
	}
	span.SetAttributes(attribute.Int("recordings.count", len(recordings)))
	addPaginationHeaders(w, r, limit, offset, len(recordings))
	writeJSON(w, http.StatusOK, map[string]any{
		"recordings": recordings,
		"limit":      limit,
		"offset":     offset,
	})
}

func (s *Server) handleGetRecordingChunk(w http.ResponseWriter, r *http.Request) {
	projectID, recordingID, ok := recordingPathIDs(w, r)
	if !ok {
		return
	}
	index, ok := chunkIndexParam(w, r)
	if !ok {
		return
	}

	ctx, span := startRecordingSpan(r, "recordings.get_chunk", projectID, recordingID,
		attribute.Int("chunk.index", index))
	defer span.End()

	s.serveRecordingChunk(w, ctx, span, projectID, recordingID, index, "fetch recording chunk failed")
}

func (s *Server) handleDeleteRecording(w http.ResponseWriter, r *http.Request) {
	projectID, recordingID, ok := recordingPathIDs(w, r)
	if !ok {
		return
	}

	ctx, span := startRecordingSpan(r, "recordings.delete", projectID, recordingID)
	defer span.End()

	if err := s.recordings.DeleteRecording(ctx, projectID, recordingID); err != nil {
		failRecording(w, span, err, "handleDeleteRecording")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetRecordingFlags(w http.ResponseWriter, r *http.Request) {
	projectID, recordingID, ok := recordingPathIDs(w, r)
	if !ok {
		return
	}

	ctx, span := startRecordingSpan(r, "recordings.get_flags", projectID, recordingID)
	defer span.End()

	sessionID, err := s.recordings.GetRecordingSessionID(ctx, projectID, recordingID)
	if err != nil {
		failRecording(w, span, err, "handleGetRecordingFlags")
		return
	}
	span.SetAttributes(attribute.String("session.id", sessionID))

	evals, err := s.recordings.FlagEvaluationsForSession(ctx, projectID, sessionID)
	if err != nil {
		failRecording(w, span, err, "handleGetRecordingFlags")
		return
	}
	if evals == nil {
		evals = []repository.FlagEvaluationEntry{}
	}
	span.SetAttributes(attribute.Int("evaluations.count", len(evals)))
	writeJSON(w, http.StatusOK, map[string]any{"evaluations": evals})
}

// fullScopeProject resolves the API key to its project and requires a
// full-scope key. Ingest-scoped keys are the public ones embedded in a site's
// page source, and replay data and trace lookups are private reads.
func (s *Server) fullScopeProject(w http.ResponseWriter, r *http.Request) (string, bool) {
	projectID, scope, ok := s.ingest.APIKeyProjectScope(r)
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	if scope != repository.APIKeyScopeFull {
		jsonError(w, "forbidden: full-scope API key required", http.StatusForbidden)
		return "", false
	}
	return projectID, true
}

// handleLookupTrace resolves a W3C trace_id (as seen in SpanBarn/BugBarn) to the
// FunnelBarn session + recording that captured it, plus the seek offset. This is
// the cross-stack deep-link: given an error trace, find the replayable session.
// Auth is by API key (project scope) so a trace only resolves within its project.
func (s *Server) handleLookupTrace(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.fullScopeProject(w, r)
	if !ok {
		return
	}
	traceID := r.PathValue("trace_id")
	if traceID == "" {
		jsonError(w, "trace_id required", http.StatusBadRequest)
		return
	}

	ctx, span := startRecordingSpan(r, "recordings.lookup_trace", projectID, "",
		attribute.String("trace.id", traceID))
	defer span.End()

	lookup, found, err := s.recordings.LookupTrace(ctx, projectID, traceID)
	if err != nil {
		failRecording(w, span, err, "handleLookupTrace")
		return
	}
	span.SetAttributes(attribute.Bool("trace.found", found))
	if !found {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, lookup)
}

// handleGetRecordingChunkByKey serves a recording chunk to an API-key client
// (e.g. the replay CLI), scoped to the key's project. It mirrors the session-auth
// chunk endpoint but lets a programmatic consumer fetch replay data with the same
// credential it used for the trace lookup. GetChunk verifies project ownership.
func (s *Server) handleGetRecordingChunkByKey(w http.ResponseWriter, r *http.Request) {
	projectID, ok := s.fullScopeProject(w, r)
	if !ok {
		return
	}
	recordingID := r.PathValue("rid")
	if recordingID == "" || r.PathValue("index") == "" {
		jsonError(w, "recording id and chunk index are required", http.StatusBadRequest)
		return
	}
	index, ok := chunkIndexParam(w, r)
	if !ok {
		return
	}

	ctx, span := startRecordingSpan(r, "recordings.get_chunk_by_key", projectID, recordingID,
		attribute.Int("chunk.index", index))
	defer span.End()

	s.serveRecordingChunk(w, ctx, span, projectID, recordingID, index, "fetch recording chunk (api key) failed")
}

// handleGetRecordingTraces returns the ordered trace timeline for a recording so
// the replay UI can overlay trace markers on the scrubber.
func (s *Server) handleGetRecordingTraces(w http.ResponseWriter, r *http.Request) {
	projectID, recordingID, ok := recordingPathIDs(w, r)
	if !ok {
		return
	}

	ctx, span := startRecordingSpan(r, "recordings.get_traces", projectID, recordingID)
	defer span.End()

	traces, err := s.recordings.TracesForRecording(ctx, projectID, recordingID)
	if err != nil {
		failRecording(w, span, err, "handleGetRecordingTraces")
		return
	}
	if traces == nil {
		traces = []repository.TraceLink{}
	}
	span.SetAttributes(attribute.Int("traces.count", len(traces)))
	writeJSON(w, http.StatusOK, map[string]any{"traces": traces})
}

func (s *Server) handleFunnelStepSessions(w http.ResponseWriter, r *http.Request) {
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

	// Same range and environment parameters as funnel analysis; an empty
	// environment keeps every environment.
	from, to, env := parseOverviewRange(r)

	ctx, span := startRecordingSpan(r, "recordings.funnel_step_sessions", projectID, "",
		attribute.String("funnel.id", funnelID),
		attribute.Int("funnel.step", step),
		attribute.String("environment", env),
	)
	defer span.End()

	sessionIDs, err := s.recordings.SessionsAtStep(ctx, funnelID, projectID, step, from, to, env)
	if err != nil {
		failRecording(w, span, err, "handleFunnelStepSessions")
		return
	}
	writeSessionIDs(w, span, sessionIDs)
}

func (s *Server) handleFlowPageSessions(w http.ResponseWriter, r *http.Request) {
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

	from, to, _ := parseOverviewRange(r)

	ctx, span := startRecordingSpan(r, "recordings.flow_page_sessions", projectID, "",
		attribute.String("page", page))
	defer span.End()

	sessionIDs, err := s.recordings.SessionsForPage(ctx, projectID, page, from, to)
	if err != nil {
		failRecording(w, span, err, "handleFlowPageSessions")
		return
	}
	writeSessionIDs(w, span, sessionIDs)
}

// writeSessionIDs answers a list of session ids, never null.
func writeSessionIDs(w http.ResponseWriter, span trace.Span, sessionIDs []string) {
	if sessionIDs == nil {
		sessionIDs = []string{}
	}
	span.SetAttributes(attribute.Int("session_ids.count", len(sessionIDs)))
	writeJSON(w, http.StatusOK, map[string]any{"session_ids": sessionIDs})
}
