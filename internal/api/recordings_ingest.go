package api

import (
	"context"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// recordingIngestTimeout bounds storing one chunk once its body has been read.
const recordingIngestTimeout = 30 * time.Second

// ingestChunkDetached stores a chunk whose body has been read in full, so the
// chunk survives the client going away. Browsers abort the request when the tab
// closes or the page navigates, which cancels the request context mid-upload;
// inheriting that cancellation lost the chunk and answered 500 (FUN-40, FUN-32).
func (s *Server) ingestChunkDetached(ctx context.Context, chunk service.RecordingChunk) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordingIngestTimeout)
	defer cancel()
	return s.recordings.IngestChunk(ctx, chunk)
}
