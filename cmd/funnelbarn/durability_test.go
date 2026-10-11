package main

// Durability of the reader/writer split (spec 012, #302). A reader takes
// events, recording chunks and evaluations while the writer is down, with a
// Valkey outage in the middle. When the writer starts, every row lands once.
// A second writer that gets every message again, as after a crash between
// apply and ack, adds nothing.

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

// writerProcess is the queue side of a writer: the bookkeeping bus and the
// ingest and recordings consumers, as run() wires them.
type writerProcess struct {
	commands   command.Bus
	ingest     *ingestQueue
	recordings *recordingsQueue
}

func startWriter(t *testing.T, ctx context.Context, cfg config.Config, applier *ingestApplier, recordings *service.RecordingService) *writerProcess {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	health := service.NewProjectHealthService(applier.store)
	commands, err := newCommandBus(ctx, cfg, command.Deps{
		Store:              applier.store,
		MarkFlagsEvaluated: health.MarkFlagsEvaluated,
		MarkProjectHealth:  service.ApplyHealthMark(health),
	}, logger)
	require.NoError(t, err)
	iq, err := newIngestQueue(ctx, cfg, applier, logger)
	require.NoError(t, err)
	rq, err := newRecordingsQueue(ctx, cfg, recordings, logger)
	require.NoError(t, err)
	return &writerProcess{commands: commands, ingest: iq, recordings: rq}
}

// drain applies everything queued.
func (w *writerProcess) drain(t *testing.T, ctx context.Context) {
	t.Helper()
	require.NoError(t, w.ingest.bus.Flush(ctx))
	require.NoError(t, w.recordings.bus.Flush(ctx))
	require.NoError(t, w.commands.Flush(ctx))
}

func (w *writerProcess) stop(t *testing.T, ctx context.Context) {
	t.Helper()
	require.NoError(t, w.commands.Close(ctx))
	require.NoError(t, w.ingest.Close(ctx))
	require.NoError(t, w.recordings.Close(ctx))
}

// durableRows is what the writer stored from the reader's traffic.
type durableRows struct {
	events      map[string]int // ingest id -> rows
	evaluations int
	chunks      int
	health      map[string]bool
}

func readDurableRows(t *testing.T, db *sql.DB, ingestIDs []string, recordingID, projectID string) durableRows {
	t.Helper()
	rows := durableRows{events: map[string]int{}, health: map[string]bool{}}
	for _, id := range ingestIDs {
		var n int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM events WHERE ingest_id = ?`, id).Scan(&n))
		rows.events[id] = n
	}
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM flag_evaluations`).Scan(&rows.evaluations))
	err := db.QueryRow(`SELECT chunk_count FROM recordings WHERE id = ?`, recordingID).Scan(&rows.chunks)
	if err != sql.ErrNoRows {
		require.NoError(t, err)
	}
	for _, field := range []string{"events_received", "setup_called"} {
		rows.health[field] = healthField(t, db, field, projectID)
	}
	return rows
}

// queueKeys are the Valkey lists a reader publishes to.
var queueKeys = []string{
	"funnelbarn:queue:" + ingestQueueName,
	"funnelbarn:queue:" + recordingsQueueName,
	"funnelbarn:queue:" + bookkeepingQueue,
}

func TestReaderTrafficSurvivesAStoppedWriter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	logger := slog.New(slog.DiscardHandler)

	applier, writerStore := newTestApplier(t)
	p, err := service.NewProjectService(writerStore).CreateProject(ctx, "Durable", "durable-site")
	require.NoError(t, err)
	flag, err := writerStore.CreateFlag(ctx, repository.FeatureFlag{
		ProjectID: p.ID, FlagKey: "durable_flag", Name: "Durable", FlagType: "boolean",
		Variants: `{"on":true,"off":false}`, DefaultVariant: "off", Split: `{"on":50,"off":50}`, Status: "active",
	})
	require.NoError(t, err)
	var dbPath string
	require.NoError(t, writerStore.DB().QueryRow(`SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&dbPath))

	mr := miniredis.RunT(t)
	url := "redis://" + mr.Addr() + "/0"
	readerCfg := config.Config{
		Mode: config.ModeReader, RedisQueueURL: url, DBPath: dbPath, SpoolDir: t.TempDir(),
		IngestQueueMaxLen: 100, RecordingsQueueMaxLen: 100,
	}
	writerCfg := config.Config{
		Mode: config.ModeWriter, RedisQueueURL: url, SpoolDir: t.TempDir(),
		IngestQueueMaxLen: 100, RecordingsQueueMaxLen: 100,
	}

	// The reader, on a read-only store, with nothing consuming its queues.
	readerStore, err := openStore(readerCfg)
	require.NoError(t, err)
	t.Cleanup(func() { readerStore.Close() })
	bus, err := newCommandBus(ctx, readerCfg, command.Deps{}, logger)
	require.NoError(t, err)
	reader := bus.(*readerCommandBus)
	defer func() { require.NoError(t, reader.Close(context.Background())) }()
	readerIngest, err := newIngestQueue(ctx, readerCfg, nil, logger)
	require.NoError(t, err)
	defer func() { require.NoError(t, readerIngest.Close(context.Background())) }()
	readerChunks, err := newRecordingsQueue(ctx, readerCfg, nil, logger)
	require.NoError(t, err)
	defer func() { require.NoError(t, readerChunks.Close(context.Background())) }()
	chunkStore := &memChunkStorage{data: map[string][]byte{}}
	readerRecordings := service.NewRecordingService(readerStore, readerStore, readerStore, chunkStore)
	readerRecordings.SetChunkQueue(readerChunks)
	readerHealth := service.NewProjectHealthService(service.HealthViaCommands{ProjectHealthRepo: readerStore, Bus: reader})

	// Events: spool, then forwarded to the ingest queue.
	sp, err := spool.New(readerCfg.SpoolDir)
	require.NoError(t, err)
	defer sp.Close()
	ingestIDs := []string{"dur-1", "dur-2", "dur-3"}
	for _, id := range ingestIDs {
		require.NoError(t, sp.Append(applyRecord(id, "durable-site", `{"name":"pageview","url":"https://example.com/","session_id":"s1"}`)))
	}
	entries, err := spool.ReadRecordsFrom(spool.Path(readerCfg.SpoolDir), 0)
	require.NoError(t, err)
	require.Len(t, entries, len(ingestIDs))
	end := readerIngest.forwarder.forward(ctx, entries, 0)
	require.Equal(t, entries[len(entries)-1].EndOffset, end)
	_, err = readerHealth.MarkEventsReceived(ctx, p.ID)
	require.NoError(t, err)

	// Recording chunks: uploaded, metadata queued.
	chunk := testChunk(p.ID)
	chunk.RecordingID = "rec-durable"
	require.NoError(t, readerRecordings.IngestChunk(ctx, chunk))
	chunk.ChunkIndex = 1
	require.NoError(t, readerRecordings.IngestChunk(ctx, chunk))

	// Evaluations: two published, one buffered on disk while Valkey is down.
	eval := command.RecordEvaluation{Eval: repository.FlagEvaluation{FlagID: flag.ID, ProjectID: p.ID, Variant: "on"}}
	reader.Submit(ctx, eval)
	reader.Submit(ctx, eval)
	mr.SetError("LOADING Valkey is down")
	reader.Submit(ctx, eval)
	reader.Submit(ctx, command.MarkProjectHealth{ProjectID: p.ID, Field: command.HealthSetupCalled})
	mr.SetError("")
	reader.fallback.forward(ctx)
	require.False(t, reader.fallback.failing)

	// The writer is down: nothing is stored, everything waits in Valkey.
	before := readDurableRows(t, writerStore.DB(), ingestIDs, "rec-durable", p.ID)
	require.Equal(t, map[string]int{"dur-1": 0, "dur-2": 0, "dur-3": 0}, before.events)
	require.Zero(t, before.evaluations)
	require.Zero(t, before.chunks)
	require.False(t, before.health["events_received"])
	queued := map[string][]string{}
	for _, key := range queueKeys {
		queued[key], err = mr.List(key)
		require.NoError(t, err)
	}
	require.Len(t, queued[queueKeys[0]], 3, "ingest records queued")
	require.Len(t, queued[queueKeys[1]], 2, "recording chunks queued")
	require.Len(t, queued[queueKeys[2]], 5, "bookkeeping commands queued")

	// The writer starts and drains the queues.
	recordings := service.NewRecordingService(writerStore, writerStore, writerStore, chunkStore)
	writer := startWriter(t, ctx, writerCfg, applier, recordings)
	writer.drain(t, ctx)
	want := durableRows{
		events:      map[string]int{"dur-1": 1, "dur-2": 1, "dur-3": 1},
		evaluations: 3,
		chunks:      2,
		health:      map[string]bool{"events_received": true, "setup_called": true},
	}
	require.Equal(t, want, readDurableRows(t, writerStore.DB(), ingestIDs, "rec-durable", p.ID))
	writer.stop(t, ctx)

	// The writer crashed after applying and before acking: the next one finds
	// every message on the processing lists and applies it again.
	for _, key := range queueKeys {
		for _, payload := range queued[key] {
			_, err := mr.Lpush(key+":processing", payload)
			require.NoError(t, err)
		}
	}
	writer = startWriter(t, ctx, writerCfg, applier, recordings)
	writer.drain(t, ctx)
	require.Equal(t, want, readDurableRows(t, writerStore.DB(), ingestIDs, "rec-durable", p.ID))
	writer.stop(t, ctx)
	// Redis deletes an empty list, so an existing key holds unacked messages.
	for _, key := range queueKeys {
		require.False(t, mr.Exists(key), "%s still queued", key)
		require.False(t, mr.Exists(key+":processing"), "%s left unacked", key)
	}
}
