package api

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// goldenInternalToken is the token the reader and the writer share in the
// reader-mode golden run.
const goldenInternalToken = "golden-internal-token"

// readerGoldenServer is a reader in front of a writer, the way the two
// Deployments run (spec 012). The reader opens the database file read-only and
// forwards every request that writes it to the writer over HTTP. Both submit
// their commands to the writer's dispatcher, which stands in for Valkey: the
// queue itself is covered by cmd/funnelbarn's mode tests.
func readerGoldenServer(t *testing.T, deps goldenDeps) goldenTarget {
	t.Helper()
	commands := newTestDispatcher(t, deps.Store)
	writerRecordings := service.NewRecordingService(deps.Store, deps.Store, deps.Store, deps.Recordings)
	writer := newGoldenServer(t, deps.Store, deps.Store, deps.Recordings, commands,
		ServerConfig{InternalToken: goldenInternalToken, Recordings: writerRecordings})
	writerHTTP := httptest.NewServer(writer)
	t.Cleanup(writerHTTP.Close)

	ro, err := repository.OpenReadOnly(deps.Path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	t.Cleanup(func() { ro.Close() })
	readerRecordings := service.NewRecordingService(ro, ro, ro, deps.Recordings)
	readerRecordings.SetChunkQueue(applyOnWriter{writerRecordings})
	reader := newGoldenServer(t, ro, service.HealthViaCommands{ProjectHealthRepo: ro, Bus: commands}, deps.Recordings, commands,
		ServerConfig{WriterURL: writerHTTP.URL, InternalToken: goldenInternalToken, Recordings: readerRecordings})

	// Sessions are rows, so the writer mints them; the reader shares its
	// session secret and derives the same CSRF tokens.
	cookie := sessionCookieFor(t, writer, goldenSessionUser)
	spare := sessionCookieFor(t, writer, goldenSessionUser)
	return goldenTarget{
		Handler: reader, Session: cookie, CSRF: reader.sessionManager.CSRFToken(cookie.Value),
		Spare: spare, SpareCSRF: reader.sessionManager.CSRFToken(spare.Value),
		Flush: commands.Flush,
	}
}

// applyOnWriter is the reader's recordings queue with the writer consuming it
// at once, as the Valkey "recordings" queue does in cmd/funnelbarn.
type applyOnWriter struct{ writer *service.RecordingService }

func (q applyOnWriter) Enqueue(ctx context.Context, meta service.ChunkMeta) bool {
	return q.writer.ApplyChunkMeta(ctx, meta) == nil
}

// TestGoldenReaderMode runs every golden case against a reader and compares
// with the same files as TestGoldenAPI: a reader with its writer answers
// exactly as a standalone process does.
func TestGoldenReaderMode(t *testing.T) {
	if *update {
		t.Skip("the golden files are written by TestGoldenAPI")
	}
	runGoldenCases(t, readerGoldenServer)
}
