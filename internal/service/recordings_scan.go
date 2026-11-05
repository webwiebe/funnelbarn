package service

import (
	"context"
	"fmt"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/enrich"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// ChunkURLScan counts, for one project, the stored chunks whose rrweb events
// hold a URL with a sensitive parameter value. It carries counts and times
// only: no keys, no event content.
type ChunkURLScan struct {
	Recordings         int        `json:"recordings"`
	Chunks             int        `json:"chunks"`
	Unreadable         int        `json:"unreadable"`
	MatchingChunks     int        `json:"matching_chunks"`
	MatchingRecordings int        `json:"matching_recordings"`
	FirstMatch         *time.Time `json:"first_match,omitempty"`
	LastMatch          *time.Time `json:"last_match,omitempty"`
}

// ScanChunkURLs reads every chunk of every recording in a project and counts
// the chunks that enrich.RedactURLsInJSON would change. It only reads. A chunk
// that cannot be fetched, decompressed or decoded counts as unreadable; a
// recording written before chunk indices were tracked may list indices it
// never stored.
func (svc *RecordingService) ScanChunkURLs(ctx context.Context, projectID string) (ChunkURLScan, error) {
	const batch = 200
	var out ChunkURLScan
	for offset := 0; ; offset += batch {
		recs, err := svc.store.ListRecordings(ctx, projectID, repository.RecordingListOpts{Limit: batch, Offset: offset})
		if err != nil {
			return out, fmt.Errorf("recordings: list for chunk scan: %w", err)
		}
		for _, rec := range recs {
			if err := svc.scanRecording(ctx, rec, &out); err != nil {
				return out, err
			}
		}
		if len(recs) < batch {
			return out, nil
		}
	}
}

// scanRecording adds one recording's chunks to out.
func (svc *RecordingService) scanRecording(ctx context.Context, rec repository.Recording, out *ChunkURLScan) error {
	out.Recordings++
	upper := rec.LastChunkIndex
	if upper < rec.ChunkCount-1 {
		upper = rec.ChunkCount - 1
	}
	matched := false
	for i := 0; i <= upper; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		out.Chunks++
		switch svc.scanChunk(ctx, chunkKey(rec.ProjectID, rec.ID, i)) {
		case chunkUnreadable:
			out.Unreadable++
		case chunkMatches:
			out.MatchingChunks++
			matched = true
		}
	}
	if matched {
		out.addMatch(rec.StartedAt)
	}
	return nil
}

type chunkScanResult int

const (
	chunkClean chunkScanResult = iota
	chunkMatches
	chunkUnreadable
)

// scanChunk reports whether the chunk at key holds a sensitive URL parameter
// value, or that it could not be read.
func (svc *RecordingService) scanChunk(ctx context.Context, key string) chunkScanResult {
	compressed, err := svc.storage.Get(ctx, key)
	if err != nil {
		return chunkUnreadable
	}
	events, err := gunzip(compressed)
	if err != nil {
		return chunkUnreadable
	}
	_, changed, err := enrich.RedactURLsInJSON(events)
	if err != nil {
		return chunkUnreadable
	}
	if changed {
		return chunkMatches
	}
	return chunkClean
}

// addMatch counts a matching recording and widens the first/last window.
func (s *ChunkURLScan) addMatch(started time.Time) {
	s.MatchingRecordings++
	if s.FirstMatch == nil || started.Before(*s.FirstMatch) {
		s.FirstMatch = &started
	}
	if s.LastMatch == nil || started.After(*s.LastMatch) {
		s.LastMatch = &started
	}
}
