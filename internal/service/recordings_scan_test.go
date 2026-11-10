package service_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/enrich"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// bigChunk is a gzipped rrweb-shaped event array of about size bytes. With
// match set, its last event carries a URL with a sensitive parameter.
func bigChunk(t *testing.T, size int, match bool) []byte {
	t.Helper()
	var b strings.Builder
	b.WriteString("[")
	for i := 0; b.Len() < size; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"type":3,"data":{"source":2,"href":"https://app.example.com/page/%d?tab=overview#s","x":%d,"y":%d,"text":"node %d"},"timestamp":%d}`, i, i%1920, i%1080, i, 1730000000000+i)
	}
	if match {
		b.WriteString(`,{"type":4,"data":{"href":"https://id.example.com/verify?token=one-time"},"timestamp":1}`)
	}
	b.WriteString("]")
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte(b.String()))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// seedRecordings stores recordings with chunks of the given payloads: every
// recording gets len(chunks) chunks, chunk i holding chunks[i].
func seedRecordings(t *testing.T, recordings int, chunks [][]byte) (*service.RecordingService, string) {
	t.Helper()
	ctx := context.Background()
	store := newTestStore(t)
	storage := newMemStorage()
	p, err := service.NewProjectService(store).CreateProject(ctx, "Scan", "scan")
	require.NoError(t, err)
	svc := service.NewRecordingService(store, store, store, storage)
	start := time.Now().UTC().Truncate(time.Second)
	for r := 0; r < recordings; r++ {
		id := fmt.Sprintf("rec-big-%03d", r)
		for i := range chunks {
			require.NoError(t, svc.IngestChunk(ctx, service.RecordingChunk{
				RecordingID: id, SessionID: "sess-" + id, ChunkIndex: i,
				Events:    json.RawMessage(`[{"type":3,"data":{},"timestamp":1}]`),
				StartedAt: start, DurationMs: 1000, Environment: "testing",
				ProjectSlug: "scan", ProjectID: p.ID, UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0)",
			}))
			// Replace the stored object with the large payload, as a chunk
			// written before ingest redaction would be.
			require.NoError(t, storage.Put(ctx, fmt.Sprintf("recordings/%s/%s/%05d.json.gz", p.ID, id, i), chunks[i]))
		}
	}
	return svc, p.ID
}

// A scan over large chunks stays under a 64 MiB memory cap: chunks are
// streamed and released, so memory tracks the largest token, not the chunk.
// Decoding a 6 MiB chunk as a whole held about 60 MiB of heap.
func TestScanChunkURLs_LargeSetStaysUnderMemoryCap(t *testing.T) {
	const capBytes = 64 << 20
	clean := bigChunk(t, 6<<20, false)
	match := bigChunk(t, 6<<20, true)
	svc, projectID := seedRecordings(t, 3, [][]byte{clean, clean, match})

	prev := debug.SetMemoryLimit(capBytes)
	defer debug.SetMemoryLimit(prev)
	runtime.GC()

	res, err := svc.ScanChunkURLs(context.Background(), projectID, service.ChunkScanOpts{BatchSize: 2, MemoryCap: capBytes})
	require.NoError(t, err)
	assert.Equal(t, 3, res.Recordings)
	assert.Equal(t, 9, res.Chunks)
	assert.Equal(t, 0, res.Unreadable)
	assert.Equal(t, 3, res.MatchingChunks)
	assert.Equal(t, 3, res.MatchingRecordings)
	assert.NotZero(t, res.PeakMemoryBytes)
	assert.Less(t, res.PeakMemoryBytes, uint64(capBytes))
}

// Past its cap the scan stops after the chunk it was on and returns the
// counts so far.
func TestScanChunkURLs_StopsAtMemoryCap(t *testing.T) {
	match := bigChunk(t, 1<<10, true)
	svc, projectID := seedRecordings(t, 2, [][]byte{match, match})

	res, err := svc.ScanChunkURLs(context.Background(), projectID, service.ChunkScanOpts{MemoryCap: 1})
	require.True(t, errors.Is(err, service.ErrMemoryCap), "want ErrMemoryCap, got %v", err)
	assert.Equal(t, 1, res.Recordings)
	assert.Equal(t, 1, res.Chunks)
	assert.Equal(t, 1, res.MatchingChunks)
	assert.Equal(t, 1, res.MatchingRecordings)
}

// gzipChunk gzips one rrweb-shaped event array.
func gzipChunk(t *testing.T, events string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte(events))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// With Shapes set, a chunk that holds token values yields only host, masked
// path and parameter name: no value and no identifier segment reaches the
// result or its JSON.
func TestScanChunkURLs_ShapesAreMaskedAndHoldNoValue(t *testing.T) {
	const (
		secret = "tok_SECRETVALUE_4f9a2c"
		pathID = "9f8e7d6c5b4a3928"
	)
	first := gzipChunk(t, `[`+
		`{"type":4,"data":{"href":"https://id.example.com/reset/`+pathID+`/confirm?token=`+secret+`&lang=nl"},"timestamp":1},`+
		`{"type":3,"data":{"href":"https://id.example.com/reset/`+pathID+`/confirm?token=`+secret+`"},"timestamp":2},`+
		`{"type":3,"data":{"src":"https://app.example.com/page?tab=overview"},"timestamp":3}]`)
	second := gzipChunk(t, `[{"type":4,"data":{"href":"https://app.example.com/#/invite/12345?invite=`+secret+`"},"timestamp":4}]`)
	clean := gzipChunk(t, `[{"type":3,"data":{"href":"https://app.example.com/page?tab=overview"},"timestamp":5}]`)
	svc, projectID := seedRecordings(t, 2, [][]byte{first, second, clean})

	res, err := svc.ScanChunkURLs(context.Background(), projectID, service.ChunkScanOpts{Shapes: true})
	require.NoError(t, err)
	assert.Equal(t, 6, res.Chunks)
	assert.Equal(t, 4, res.MatchingChunks)
	assert.Equal(t, 2, res.MatchingRecordings)

	reset := enrich.URLShape{Host: "id.example.com", Path: "/reset/:id/confirm", Param: "token"}
	invite := enrich.URLShape{Host: "app.example.com", Path: "/#/invite/:id", Param: "invite"}
	assert.ElementsMatch(t, []service.URLShapeCount{
		{URLShape: reset, Chunks: 2, Recordings: 2},
		{URLShape: invite, Chunks: 2, Recordings: 2},
	}, res.Shapes)

	out, err := json.Marshal(res)
	require.NoError(t, err)
	for _, leak := range []string{secret, "SECRET", pathID, "12345", "lang", "overview"} {
		assert.NotContains(t, string(out), leak)
	}
	assert.Contains(t, string(out), `"path":"/reset/:id/confirm"`)
}

// Past MaxShapes distinct shapes, the rest count under OtherShapes.
func TestScanChunkURLs_ShapesCapAtMaxShapes(t *testing.T) {
	chunk := gzipChunk(t, `[{"href":"https://a.example/one?token=x"},{"href":"https://a.example/two?token=x"},{"href":"https://a.example/three?token=x"}]`)
	svc, projectID := seedRecordings(t, 1, [][]byte{chunk})

	res, err := svc.ScanChunkURLs(context.Background(), projectID, service.ChunkScanOpts{Shapes: true, MaxShapes: 2})
	require.NoError(t, err)
	require.Len(t, res.Shapes, 3)
	var other *service.URLShapeCount
	for i := range res.Shapes {
		if res.Shapes[i].URLShape == service.OtherShapes {
			other = &res.Shapes[i]
		}
	}
	require.NotNil(t, other)
	assert.Equal(t, 1, other.Chunks)
}

// Without Shapes the result carries no shapes.
func TestScanChunkURLs_NoShapesByDefault(t *testing.T) {
	svc, projectID := seedRecordings(t, 1, [][]byte{bigChunk(t, 1<<10, true)})
	res, err := svc.ScanChunkURLs(context.Background(), projectID, service.ChunkScanOpts{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.MatchingChunks)
	assert.Empty(t, res.Shapes)
}

// A run that is cut off leaves the counts reported so far: Progress gets a
// growing copy of them while the project is read, and never any shapes.
func TestScanChunkURLs_ReportsProgress(t *testing.T) {
	match := bigChunk(t, 1<<10, true)
	svc, projectID := seedRecordings(t, 3, [][]byte{match, match})

	var seen []service.ChunkURLScan
	res, err := svc.ScanChunkURLs(context.Background(), projectID, service.ChunkScanOpts{
		Shapes:        true,
		ProgressEvery: time.Nanosecond,
		Progress:      func(part service.ChunkURLScan) { seen = append(seen, part) },
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(seen), 2, "the first chunk starts the clock, later chunks report")
	for i, part := range seen {
		assert.Empty(t, part.Shapes, "progress carries counts only")
		assert.LessOrEqual(t, part.Chunks, res.Chunks)
		if i > 0 {
			assert.Greater(t, part.Chunks, seen[i-1].Chunks)
		}
	}
	assert.Equal(t, 6, res.Chunks)
}

// Without a Progress function nothing is reported and nothing breaks.
func TestScanChunkURLs_NoProgressByDefault(t *testing.T) {
	svc, projectID := seedRecordings(t, 1, [][]byte{bigChunk(t, 1<<10, false)})
	res, err := svc.ScanChunkURLs(context.Background(), projectID, service.ChunkScanOpts{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Chunks)
}
