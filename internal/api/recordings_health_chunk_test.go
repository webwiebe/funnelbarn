package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// markCounter counts MarkRecordingsReceived calls; the rest of the health
// service is the real one.
type markCounter struct {
	service.ProjectHealth
	marks chan string
}

func (m *markCounter) MarkRecordingsReceived(_ context.Context, projectID string) error {
	m.marks <- projectID
	return nil
}

// drainMarks returns the projects marked within the window. The old behaviour
// marked from a goroutine, so the window gives it time to show up.
func drainMarks(m *markCounter, window time.Duration) []string {
	var got []string
	timer := time.After(window)
	for {
		select {
		case id := <-m.marks:
			got = append(got, id)
		case <-timer:
			return got
		}
	}
}

func validChunk(id string) map[string]any {
	return map[string]any{
		"recording_id": id,
		"session_id":   "s" + id,
		"chunk_index":  0,
		"events":       []map[string]any{{"type": 2, "data": map[string]any{}}},
		"started_at":   time.Now().UTC(),
		"duration_ms":  10,
	}
}

// A project whose chunks are all rejected must not look like it is receiving
// recordings: the health page would hide the broken setup.
func TestIngestRecordingChunk_RejectedChunkDoesNotMarkHealth(t *testing.T) {
	srv, store := newRecordingServer(t)
	health := &markCounter{ProjectHealth: service.NewProjectHealthService(store), marks: make(chan string, 8)}
	srv.projectHealth = health
	projectID, key := projectKey(t, store, "Health", "health-mark")

	// No events: rejected with 422.
	bad := validChunk("recBad")
	delete(bad, "events")
	if w := postChunk(t, srv, key, bad); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for an empty chunk, got %d: %s", w.Code, w.Body.String())
	}
	// Unsafe id: rejected with 422.
	bad = validChunk("rec/../x")
	if w := postChunk(t, srv, key, bad); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for an unsafe id, got %d: %s", w.Code, w.Body.String())
	}
	if got := drainMarks(health, 200*time.Millisecond); len(got) != 0 {
		t.Fatalf("rejected chunks marked health: %v", got)
	}

	// An accepted chunk marks it.
	if w := postChunk(t, srv, key, validChunk("recGood")); w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d: %s", w.Code, w.Body.String())
	}
	got := drainMarks(health, 500*time.Millisecond)
	if len(got) != 1 || got[0] != projectID {
		t.Fatalf("want one mark for %s after an accepted chunk, got %v", projectID, got)
	}
}

// failingGetStorage stores normally and fails every read with err.
type failingGetStorage struct {
	*memStorage
	err error
}

func (s *failingGetStorage) Get(context.Context, string) ([]byte, error) { return nil, s.err }

// A storage failure whose text happens to hold "404" (a request id, a port) is
// a server error, not a missing chunk.
func TestGetRecordingChunk_StorageErrorContaining404IsServerError(t *testing.T) {
	h := captureSlog(t)
	storage := &failingGetStorage{
		memStorage: newMemStorage(),
		err:        fmt.Errorf("r2 get: operation error S3: GetObject, https response error StatusCode: 500, RequestID: 40412345, api error InternalError"),
	}
	srv, store := newRecordingServerWithStorage(t, storage)
	projectID, ingestKey := projectKey(t, store, "Chunk 404", "chunk-404")
	readKey := fullKey(t, store, projectID, "chunk-404")
	if w := postChunk(t, srv, ingestKey, validChunk("recFetch")); w.Code != http.StatusAccepted {
		t.Fatalf("seed chunk: %d %s", w.Code, w.Body.String())
	}

	w := getChunkByKey(t, srv, readKey, "recFetch", 0)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d: %s", w.Code, w.Body.String())
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.recs {
		if r.Level == slog.LevelError && r.Message == "fetch recording chunk (api key) failed" {
			return
		}
	}
	t.Fatal("want an error log line for the failed fetch")
}

func TestIsChunkNotFound(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"domain not found", fmt.Errorf("wrap: %w", domain.ErrNotFound), true},
		{"s3 NoSuchKey", fmt.Errorf("r2 get: %w", &types.NoSuchKey{}), true},
		{"s3 NotFound", fmt.Errorf("r2 get: %w", &types.NotFound{}), true},
		{"http 404 response", fmt.Errorf("r2 get: %w", statusErr(404)), true},
		{"http 500 with 404 in the request id", fmt.Errorf("RequestID: 40412345: %w", statusErr(500)), false},
		{"plain text with 404", errors.New("dial tcp 10.0.0.1:404: connection refused"), false},
		{"plain text NoSuchKey", errors.New("NoSuchKey"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isChunkNotFound(c.err); got != c.want {
				t.Fatalf("isChunkNotFound(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

// statusErr stands in for the SDK's HTTP response error, which exposes the
// status through HTTPStatusCode.
type statusErr int

func (e statusErr) Error() string       { return fmt.Sprintf("https response error StatusCode: %d", int(e)) }
func (e statusErr) HTTPStatusCode() int { return int(e) }
