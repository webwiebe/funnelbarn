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
