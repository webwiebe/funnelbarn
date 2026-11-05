package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"runtime/metrics"
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
	// PeakMemoryBytes is the most memory the Go runtime held from the OS at
	// any check between chunks.
	PeakMemoryBytes uint64 `json:"peak_memory_bytes"`
}

// ChunkScanOpts bounds a chunk scan.
type ChunkScanOpts struct {
	// BatchSize is the number of recordings listed per page. Zero means 25.
	BatchSize int
	// MemoryCap stops the scan when, after a chunk, the Go runtime holds more
	// than this many bytes from the OS even after returning what it can. Zero
	// means no cap. Pair it with debug.SetMemoryLimit at the same size, so the
	// runtime collects before the cap is reached.
	MemoryCap uint64
}

// ErrMemoryCap means a chunk scan stopped because its memory passed the cap.
// The counts returned with it cover the chunks read before the stop.
var ErrMemoryCap = errors.New("recordings: chunk scan stopped at its memory cap")

// ScanChunkURLs reads every chunk of every recording in a project and counts
// the chunks that enrich.RedactURLsInJSON would change. It only reads. A chunk
// that cannot be fetched, decompressed or decoded counts as unreadable; a
// recording written before chunk indices were tracked may list indices it
// never stored.
//
// Recordings are listed in pages of opts.BatchSize and each chunk is streamed
// through enrich.HasSensitiveURLInJSON, so no chunk is decoded into memory as
// a whole and nothing from it is kept after its check. After every chunk the
// process memory is measured against opts.MemoryCap; past the cap the scan
// returns the counts so far with ErrMemoryCap.
func (svc *RecordingService) ScanChunkURLs(ctx context.Context, projectID string, opts ChunkScanOpts) (ChunkURLScan, error) {
	batch := opts.BatchSize
	if batch <= 0 {
		batch = 25
	}
	var out ChunkURLScan
	for offset := 0; ; offset += batch {
		recs, err := svc.store.ListRecordings(ctx, projectID, repository.RecordingListOpts{Limit: batch, Offset: offset})
		if err != nil {
			return out, fmt.Errorf("recordings: list for chunk scan: %w", err)
		}
		for _, rec := range recs {
			if err := svc.scanRecording(ctx, rec, opts.MemoryCap, &out); err != nil {
				return out, err
			}
		}
		if len(recs) < batch {
			return out, nil
		}
	}
}

// scanRecording adds one recording's chunks to out.
func (svc *RecordingService) scanRecording(ctx context.Context, rec repository.Recording, memCap uint64, out *ChunkURLScan) error {
	out.Recordings++
	upper := rec.LastChunkIndex
	if upper < rec.ChunkCount-1 {
		upper = rec.ChunkCount - 1
	}
	matched := false
	defer func() {
		if matched {
			out.addMatch(rec.StartedAt)
		}
	}()
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
		if err := out.checkMemory(memCap); err != nil {
			return err
		}
	}
	return nil
}

// checkMemory records the process memory and reports ErrMemoryCap when it
// stays above limit after the runtime returns what it can to the OS. The peak
// is the reading before that return, so a chunk that briefly needed more
// memory still shows in the output.
func (s *ChunkURLScan) checkMemory(limit uint64) error {
	m := goMemoryBytes()
	if m > s.PeakMemoryBytes {
		s.PeakMemoryBytes = m
	}
	if limit == 0 || m <= limit {
		return nil
	}
	debug.FreeOSMemory()
	if m = goMemoryBytes(); m > limit {
		return fmt.Errorf("%w: %d bytes over a cap of %d", ErrMemoryCap, m, limit)
	}
	return nil
}

// goMemoryBytes is the memory the Go runtime holds from the OS: everything it
// mapped minus the heap pages it already returned. This is the figure
// debug.SetMemoryLimit controls, and close to what the container sees.
// runtime/metrics reads it without stopping the world.
func goMemoryBytes() uint64 {
	sample := []metrics.Sample{
		{Name: "/memory/classes/total:bytes"},
		{Name: "/memory/classes/heap/released:bytes"},
	}
	metrics.Read(sample)
	if sample[0].Value.Kind() != metrics.KindUint64 || sample[1].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return sample[0].Value.Uint64() - sample[1].Value.Uint64()
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
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return chunkUnreadable
	}
	defer func() { _ = zr.Close() }()
	changed, err := enrich.HasSensitiveURLInJSON(zr)
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
