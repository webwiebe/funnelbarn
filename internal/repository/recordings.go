package repository

import (
	"context"
	"time"
)

// RecordingChunk holds one chunk of an rrweb session recording.
type RecordingChunk struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	SessionID  string    `json:"session_id"`
	ChunkIndex int       `json:"chunk_index"`
	EventsJSON string    `json:"-"`
	ReceivedAt time.Time `json:"received_at"`
}

// InsertRecordingChunk stores an rrweb recording chunk.
func (s *Store) InsertRecordingChunk(ctx context.Context, c RecordingChunk) error {
	const q = `
		INSERT INTO recording_chunks (id, project_id, session_id, chunk_index, events_json, received_at)
		VALUES (?, ?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, q, c.ID, c.ProjectID, c.SessionID, c.ChunkIndex, c.EventsJSON, c.ReceivedAt)
	return err
}
