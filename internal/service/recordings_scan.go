package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"runtime/metrics"
	"sort"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/enrich"
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
	// Shapes lists where the matching URLs pointed, when the scan collects
	// them (ChunkScanOpts.Shapes): host, masked path and parameter name, with
	// counts, most recordings first. It never holds a parameter value.
	Shapes []URLShapeCount `json:"shapes,omitempty"`

	tally    map[enrich.URLShape]*URLShapeCount
	reported time.Time // when Progress last ran
}

// URLShapeCount counts the chunks and recordings in which one URL shape held
// a sensitive parameter value.
type URLShapeCount struct {
	enrich.URLShape
	Chunks     int `json:"chunks"`
	Recordings int `json:"recordings"`
}

// OtherShapes is the shape that counts every shape past ChunkScanOpts.MaxShapes.
var OtherShapes = enrich.URLShape{Host: "(other)", Path: "(other)", Param: "(other)"}

// ChunkScanOpts bounds a chunk scan.
type ChunkScanOpts struct {
	// BatchSize is the number of recordings listed per page. Zero means 25.
	BatchSize int
	// MemoryCap stops the scan when, after a chunk, the Go runtime holds more
	// than this many bytes from the OS even after returning what it can. Zero
	// means no cap. Pair it with debug.SetMemoryLimit at the same size, so the
	// runtime collects before the cap is reached.
	MemoryCap uint64
	// Shapes makes the scan read every chunk to its end and count each
	// matching URL by host, masked path and parameter name (enrich.URLShape).
	Shapes bool
	// MaxShapes bounds the distinct shapes kept per project; the rest count
	// under OtherShapes. Zero means 500.
	MaxShapes int
	// Progress, when set, receives a copy of the counts so far (without
	// shapes) at most once per ProgressEvery, checked after each chunk. A run
	// that is cut off still leaves these counts behind.
	Progress func(ChunkURLScan)
	// ProgressEvery spaces the Progress calls. Zero means 30 seconds.
	ProgressEvery time.Duration
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
func (svc *RecordingService) ScanChunkURLs(ctx context.Context, projectID string, opts ChunkScanOpts) (out ChunkURLScan, err error) {
	batch := opts.BatchSize
	if batch <= 0 {
		batch = 25
	}
	if opts.MaxShapes <= 0 {
		opts.MaxShapes = 500
	}
	defer out.finishShapes()
	for offset := 0; ; offset += batch {
		recs, err := svc.store.ListRecordings(ctx, projectID, domain.RecordingListOpts{Limit: batch, Offset: offset})
		if err != nil {
			return out, fmt.Errorf("recordings: list for chunk scan: %w", err)
		}
		for _, rec := range recs {
			if err := svc.scanRecording(ctx, rec, opts, &out); err != nil {
				return out, err
			}
		}
		if len(recs) < batch {
			return out, nil
		}
	}
}

// scanRecording adds one recording's chunks to out.
func (svc *RecordingService) scanRecording(ctx context.Context, rec domain.Recording, opts ChunkScanOpts, out *ChunkURLScan) error {
	out.Recordings++
	upper := rec.LastChunkIndex
	if upper < rec.ChunkCount-1 {
		upper = rec.ChunkCount - 1
	}
	matched := false
	seen := map[enrich.URLShape]bool{}
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
		res, shapes := svc.scanChunk(ctx, chunkKey(rec.ProjectID, rec.ID, i), opts.Shapes)
		switch res {
		case chunkUnreadable:
			out.Unreadable++
		case chunkMatches:
			out.MatchingChunks++
			matched = true
			out.addShapes(shapes, seen, opts.MaxShapes)
		}
		if err := out.checkMemory(opts.MemoryCap); err != nil {
			return err
		}
		out.report(opts)
	}
	return nil
}

// report hands opts.Progress a copy of the counts when ProgressEvery has
// passed since the last call. The first chunk starts the clock.
func (s *ChunkURLScan) report(opts ChunkScanOpts) {
	if opts.Progress == nil {
		return
	}
	every := opts.ProgressEvery
	if every <= 0 {
		every = 30 * time.Second
	}
	now := time.Now()
	if s.reported.IsZero() {
		s.reported = now
		return
	}
	if now.Sub(s.reported) < every {
		return
	}
	s.reported = now
	snap := *s
	snap.tally = nil
	snap.Shapes = nil
	opts.Progress(snap)
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
// value, or that it could not be read. With collect set it reads the whole
// chunk and also returns the distinct shapes of its matching URLs.
func (svc *RecordingService) scanChunk(ctx context.Context, key string, collect bool) (chunkScanResult, []enrich.URLShape) {
	compressed, err := svc.storage.Get(ctx, key)
	if err != nil {
		return chunkUnreadable, nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return chunkUnreadable, nil
	}
	defer func() { _ = zr.Close() }()
	if !collect {
		changed, err := enrich.HasSensitiveURLInJSON(zr)
		switch {
		case err != nil:
			return chunkUnreadable, nil
		case changed:
			return chunkMatches, nil
		}
		return chunkClean, nil
	}
	return collectShapes(zr)
}

// collectShapes walks a whole chunk and returns its distinct URL shapes. A
// chunk that fails to decode counts as unreadable and returns no shapes.
func collectShapes(r io.Reader) (chunkScanResult, []enrich.URLShape) {
	set := map[enrich.URLShape]bool{}
	err := enrich.WalkSensitiveURLsInJSON(r, func(u string) {
		for _, sh := range enrich.SensitiveURLShapes(u) {
			set[sh] = true
		}
	})
	if err != nil {
		return chunkUnreadable, nil
	}
	if len(set) == 0 {
		return chunkClean, nil
	}
	shapes := make([]enrich.URLShape, 0, len(set))
	for sh := range set {
		shapes = append(shapes, sh)
	}
	return chunkMatches, shapes
}

// addShapes counts one matching chunk's shapes, and each shape once per
// recording through seen. Past maxShapes distinct shapes, new ones count
// under OtherShapes.
func (s *ChunkURLScan) addShapes(shapes []enrich.URLShape, seen map[enrich.URLShape]bool, maxShapes int) {
	if len(shapes) == 0 {
		return
	}
	if s.tally == nil {
		s.tally = map[enrich.URLShape]*URLShapeCount{}
	}
	for _, sh := range shapes {
		if _, ok := s.tally[sh]; !ok && len(s.tally) >= maxShapes {
			sh = OtherShapes
		}
		c, ok := s.tally[sh]
		if !ok {
			c = &URLShapeCount{URLShape: sh}
			s.tally[sh] = c
		}
		c.Chunks++
		if !seen[sh] {
			seen[sh] = true
			c.Recordings++
		}
	}
}

// finishShapes turns the tally into Shapes, most recordings first.
func (s *ChunkURLScan) finishShapes() {
	if len(s.tally) == 0 {
		return
	}
	s.Shapes = make([]URLShapeCount, 0, len(s.tally))
	for _, c := range s.tally {
		s.Shapes = append(s.Shapes, *c)
	}
	sort.Slice(s.Shapes, func(i, j int) bool {
		a, b := s.Shapes[i], s.Shapes[j]
		if a.Recordings != b.Recordings {
			return a.Recordings > b.Recordings
		}
		if a.Chunks != b.Chunks {
			return a.Chunks > b.Chunks
		}
		return a.Host+a.Path+a.Param < b.Host+b.Path+b.Param
	})
	s.tally = nil
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
