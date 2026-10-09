package command

import (
	"context"
	"errors"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// KindApplyRecordingChunk labels a recording chunk's metadata carried on the
// recordings queue.
const KindApplyRecordingChunk = "apply_recording_chunk"

// ApplyRecordingChunk folds one uploaded recording chunk into its recording
// row. The chunk's events are already in R2; only this metadata travels on the
// queue. The store records each (recording, chunk index) once, so a
// redelivery changes nothing.
type ApplyRecordingChunk struct {
	Recording  repository.Recording   `json:"recording"`
	ChunkIndex int                    `json:"chunk_index"`
	Traces     []repository.TraceLink `json:"traces,omitempty"`
}

// Kind implements Command.
func (ApplyRecordingChunk) Kind() string { return KindApplyRecordingChunk }

// Project implements Command.
func (c ApplyRecordingChunk) Project() string { return c.Recording.ProjectID }

// Apply implements Command.
func (c ApplyRecordingChunk) Apply(ctx context.Context, d Deps) error {
	if d.ApplyChunk == nil {
		return errors.New("recording chunk: no chunk applier configured")
	}
	return d.ApplyChunk(ctx, c)
}

func (ApplyRecordingChunk) durable() {}
