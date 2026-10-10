package main

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
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

// readerWriterPair runs a writer and a reader on one database file and one
// Valkey, the way the two Deployments do.
type readerWriterPair struct {
	mr          *miniredis.Miniredis
	writerStore *repository.Store
	writer      command.Bus
	readerStore *repository.Store
	reader      *readerCommandBus
	project     repository.Project
	flag        repository.FeatureFlag
}

func newReaderWriterPair(t *testing.T) *readerWriterPair {
	t.Helper()
	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)
	writerStore, p, f := newCommandTestStore(t)
	mr := miniredis.RunT(t)
	url := "redis://" + mr.Addr() + "/0"

	health := service.NewProjectHealthService(writerStore)
	writer, err := newCommandBus(ctx, config.Config{Mode: config.ModeWriter, RedisQueueURL: url}, command.Deps{
		Store:              writerStore,
		MarkFlagsEvaluated: health.MarkFlagsEvaluated,
		MarkProjectHealth:  markProjectHealth(health),
	}, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = writer.Close(context.Background()) })

	var dbPath string
	require.NoError(t, writerStore.DB().QueryRow(`SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&dbPath))
	readerCfg := config.Config{Mode: config.ModeReader, RedisQueueURL: url, DBPath: dbPath, SpoolDir: t.TempDir()}
	readerStore, err := openStore(readerCfg)
	require.NoError(t, err)
	t.Cleanup(func() { readerStore.Close() })
	reader, err := newCommandBus(ctx, readerCfg, command.Deps{}, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close(context.Background()) })
	require.IsType(t, &readerCommandBus{}, reader)

	return &readerWriterPair{
		mr: mr, writerStore: writerStore, writer: writer,
		readerStore: readerStore, reader: reader.(*readerCommandBus),
		project: p, flag: f,
	}
}

func healthField(t *testing.T, db *sql.DB, field, projectID string) bool {
	t.Helper()
	var v bool
	err := db.QueryRow(`SELECT `+field+` FROM project_health WHERE project_id = ?`, projectID).Scan(&v)
	if err == sql.ErrNoRows {
		return false
	}
	require.NoError(t, err)
	return v
}

func TestReaderPublishesAndTheWriterApplies(t *testing.T) {
	pair := newReaderWriterPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pair.reader.Submit(ctx, command.RecordEvaluation{Eval: repository.FlagEvaluation{
		FlagID: pair.flag.ID, ProjectID: pair.project.ID, Variant: "on",
	}})
	readerHealth := service.NewProjectHealthService(healthViaCommands{ProjectHealthRepo: pair.readerStore, bus: pair.reader})
	first, err := readerHealth.MarkEventsReceived(ctx, pair.project.ID)
	require.NoError(t, err)
	require.True(t, first)

	require.NoError(t, pair.writer.Flush(ctx))
	require.Equal(t, 1, evaluationRows(t, pair.writerStore))
	require.True(t, healthField(t, pair.writerStore.DB(), "events_received", pair.project.ID))

	// The reader wrote nothing itself: its store refuses writes.
	_, err = pair.readerStore.CreateProject(ctx, "X", "x")
	require.ErrorContains(t, err, "readonly")
}

func TestReaderBuffersWhileValkeyIsDown(t *testing.T) {
	pair := newReaderWriterPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pair.mr.SetError("LOADING Valkey is down")
	pair.reader.Submit(ctx, command.RecordEvaluation{Eval: repository.FlagEvaluation{
		FlagID: pair.flag.ID, ProjectID: pair.project.ID, Variant: "on",
	}})
	pair.reader.Submit(ctx, command.MarkProjectHealth{ProjectID: pair.project.ID, Field: command.HealthSetupCalled})

	// Still down: forwarding keeps the commands on disk.
	pair.reader.fallback.forward(ctx)
	require.True(t, pair.reader.fallback.failing)

	pair.mr.SetError("")
	pair.reader.fallback.forward(ctx)
	require.False(t, pair.reader.fallback.failing)

	require.NoError(t, pair.writer.Flush(ctx))
	require.Equal(t, 1, evaluationRows(t, pair.writerStore))
	require.True(t, healthField(t, pair.writerStore.DB(), "setup_called", pair.project.ID))
}

func TestReaderQueuesDoNotConsume(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := config.Config{Mode: config.ModeReader, RedisQueueURL: "redis://" + mr.Addr() + "/0", SpoolDir: t.TempDir(), IngestQueueMaxLen: 10, RecordingsQueueMaxLen: 10}
	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)

	iq, err := newIngestQueue(ctx, cfg, nil, logger)
	require.NoError(t, err)
	require.NotNil(t, iq, "a reader always forwards ingest to the queue")
	require.Nil(t, iq.bus)
	require.NoError(t, iq.Close(ctx))

	rq, err := newRecordingsQueue(ctx, cfg, nil, logger)
	require.NoError(t, err)
	require.NotNil(t, rq.fallback)
	require.Nil(t, rq.bus)

	// Valkey down: the chunk is buffered and reported queued, so the
	// recording service does not try to apply it on the read-only store.
	mr.SetError("LOADING Valkey is down")
	require.True(t, rq.Enqueue(ctx, service.ChunkMeta{Recording: repository.Recording{ID: "r1", ProjectID: "p1"}, ChunkIndex: 0}))
	mr.SetError("")
	rq.fallback.forward(ctx)
	require.False(t, rq.fallback.failing)
	n, err := rq.list.QueuedLen(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	require.NoError(t, rq.Close(ctx))
}

func TestStandaloneIgnoresTheSplitDefaults(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)
	cfg := config.Config{Mode: config.ModeStandalone, SpoolDir: t.TempDir()}
	iq, err := newIngestQueue(ctx, cfg, nil, logger)
	require.NoError(t, err)
	require.Nil(t, iq)
	rq, err := newRecordingsQueue(ctx, cfg, nil, logger)
	require.NoError(t, err)
	require.Nil(t, rq)
}

func TestSchemaGateWaitsForTheWriter(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "gate.db")
	writer, err := repository.Open(path)
	require.NoError(t, err)
	defer writer.Close()

	// Hide the newest migrations, as if the writer has not run them yet.
	_, err = writer.DB().Exec(`CREATE TABLE hidden_versions AS SELECT * FROM goose_db_version WHERE version_id > 30`)
	require.NoError(t, err)
	_, err = writer.DB().Exec(`DELETE FROM goose_db_version WHERE version_id > 30`)
	require.NoError(t, err)

	reader, err := repository.OpenReadOnly(path)
	require.NoError(t, err)
	defer reader.Close()
	gate, err := newSchemaGate(reader.Reader())
	require.NoError(t, err)
	require.ErrorContains(t, gate.Ready(ctx), "waiting for the writer to migrate")

	// The writer migrates.
	_, err = writer.DB().Exec(`INSERT INTO goose_db_version SELECT * FROM hidden_versions`)
	require.NoError(t, err)
	require.NoError(t, gate.Ready(ctx))
}

func TestMarkProjectHealthRejectsAnUnknownField(t *testing.T) {
	store, p, _ := newCommandTestStore(t)
	mark := markProjectHealth(service.NewProjectHealthService(store))
	err := mark(context.Background(), p.ID, "no_such_field")
	require.ErrorIs(t, err, command.ErrPermanent)
	require.NoError(t, mark(context.Background(), p.ID, command.HealthRecordingsReceived))
	require.True(t, healthField(t, store.DB(), "recordings_received", p.ID))
}

func TestReaderWorkerForwardsTheSpool(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := config.Config{Mode: config.ModeReader, RedisQueueURL: "redis://" + mr.Addr() + "/0", SpoolDir: t.TempDir(), IngestQueueMaxLen: 10}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sp, err := spool.New(cfg.SpoolDir)
	require.NoError(t, err)
	defer sp.Close()
	require.NoError(t, sp.Append(spool.Record{IngestID: "i1", ReceivedAt: time.Now(), BodyBase64: "e30=", ProjectSlug: "s"}))

	iq, err := newIngestQueue(ctx, cfg, nil, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	defer iq.Close(context.Background())

	done := make(chan struct{})
	go func() {
		runReaderWorker(ctx, cfg, sp, iq, nil)
		close(done)
	}()
	require.Eventually(t, func() bool {
		n, err := iq.forwarder.list.QueuedLen(context.Background())
		return err == nil && n == 1
	}, 5*time.Second, 20*time.Millisecond)
	cancel()
	<-done
}
