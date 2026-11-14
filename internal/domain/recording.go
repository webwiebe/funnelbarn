package domain

import "time"

// Recording holds metadata for a session recording.
type Recording struct {
	ID              string     `json:"id"`
	ProjectID       string     `json:"project_id"`
	SessionID       string     `json:"session_id"`
	Environment     string     `json:"environment"`
	FirstChunkIndex int        `json:"first_chunk_index"`
	LastChunkIndex  int        `json:"last_chunk_index"`
	ChunkCount      int        `json:"chunk_count"`
	HasSnapshot     bool       `json:"has_snapshot"`
	DurationMs      int64      `json:"duration_ms"`
	StartedAt       time.Time  `json:"started_at"`
	EndedAt         *time.Time `json:"ended_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	DeviceType      string     `json:"device_type"`
	UserAgent       string     `json:"user_agent,omitempty"`
	IsBot           bool       `json:"is_bot"`
	PageURL         string     `json:"page_url,omitempty"`
}

// FlagEvaluationEntry is a flag evaluation linked to a session.
type FlagEvaluationEntry struct {
	FlagName    string    `json:"flag_name"`
	Variant     string    `json:"variant"`
	EvaluatedAt time.Time `json:"evaluated_at"`
}

// RecordingListOpts holds optional filter/pagination parameters for ListRecordings.
type RecordingListOpts struct {
	Environment string
	SessionIDs  []string
	DeviceType  string // "mobile" | "desktop" | "tablet" | ""
	HumanOnly   bool
	PageURL     string
	Limit       int
	Offset      int
}

// TraceLink is a single W3C trace observed in the browser during a recording.
// It is sent by the SDK alongside a recording chunk and stored so a trace_id from
// SpanBarn/BugBarn can be resolved back to a session + recording (and seek offset).
type TraceLink struct {
	TraceID    string    `json:"trace_id"`
	SpanID     string    `json:"span_id,omitempty"`
	URL        string    `json:"url,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// TraceLookup resolves a trace_id to the recording that captured it, including the
// seek offset (occurred_at - recording.started_at) so a replay can jump to the
// moment the trace fired.
type TraceLookup struct {
	TraceID            string    `json:"trace_id"`
	ProjectID          string    `json:"project_id"`
	SessionID          string    `json:"session_id"`
	RecordingID        string    `json:"recording_id"`
	OccurredAt         time.Time `json:"occurred_at"`
	OffsetMs           int64     `json:"offset_ms"`
	URL                string    `json:"url,omitempty"`
	RecordingStartedAt time.Time `json:"recording_started_at"`
	// Chunk range + metadata of the resolved recording, so a replay client can
	// fetch the chunks and render without a second round-trip.
	FirstChunkIndex int    `json:"first_chunk_index"`
	LastChunkIndex  int    `json:"last_chunk_index"`
	ChunkCount      int    `json:"chunk_count"`
	DurationMs      int64  `json:"duration_ms"`
	Environment     string `json:"environment,omitempty"`
	PageURL         string `json:"page_url,omitempty"`
}
