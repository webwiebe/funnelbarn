package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

const recordingMaxBodyBytes = 4 << 20 // 4 MiB

// RecordingChunkRepo is the narrow storage interface for recording chunks.
type RecordingChunkRepo interface {
	InsertRecordingChunk(ctx context.Context, c repository.RecordingChunk) error
}

// handleRecordingChunk handles POST /api/v1/recordings/chunk.
// Accepts rrweb session recording chunks from authenticated API-key clients.
func (s *Server) handleRecordingChunk(w http.ResponseWriter, r *http.Request) {
	projectID, _, ok := s.ingest.APIKeyProjectScope(r)
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Static env-var keys are not project-scoped; fall back to the header.
	if projectID == "" {
		slug := r.Header.Get("x-funnelbarn-project")
		if slug == "" {
			jsonError(w, "x-funnelbarn-project header required", http.StatusBadRequest)
			return
		}
		proj, err := s.projects.GetProjectBySlug(r.Context(), slug)
		if err != nil {
			jsonError(w, "project not found", http.StatusNotFound)
			return
		}
		projectID = proj.ID
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, recordingMaxBodyBytes))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			jsonError(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		jsonError(w, "unable to read request body", http.StatusBadRequest)
		return
	}

	var payload struct {
		SessionID  string          `json:"session_id"`
		ChunkIndex int             `json:"chunk_index"`
		Events     json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if payload.SessionID == "" {
		jsonError(w, "session_id required", http.StatusBadRequest)
		return
	}
	if len(payload.Events) == 0 || string(payload.Events) == "null" {
		jsonError(w, "events required", http.StatusBadRequest)
		return
	}

	chunk := repository.RecordingChunk{
		ID:         generateRecordingID(),
		ProjectID:  projectID,
		SessionID:  payload.SessionID,
		ChunkIndex: payload.ChunkIndex,
		EventsJSON: string(payload.Events),
		ReceivedAt: time.Now().UTC(),
	}

	if err := s.recordings.InsertRecordingChunk(r.Context(), chunk); err != nil {
		slog.Error("insert recording chunk", "session_id", chunk.SessionID, "err", err)
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	slog.Info("recording chunk stored",
		"session_id", chunk.SessionID,
		"chunk_index", chunk.ChunkIndex,
		"project_id", chunk.ProjectID,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"accepted": true})
}

func generateRecordingID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return time.Now().UTC().Format("20060102T150405.000000000Z")
	}
	return hex.EncodeToString(raw[:])
}
