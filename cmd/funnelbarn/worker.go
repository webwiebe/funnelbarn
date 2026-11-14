package main

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/bblog"
	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest/pipeline"
	"github.com/wiebe-xyz/funnelbarn/internal/maintenance"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

// startWorker starts the background worker for the process mode and returns
// the readiness gate for GET /api/v1/ready. A reader gets the worker that
// forwards its spool and fallback files to Valkey, and a gate that holds until
// the writer has migrated the database. Any other mode gets the full worker
// and no gate.
func startWorker(ctx context.Context, cfg config.Config, store *repository.Store, eventSpool *spool.Spool, applier *pipeline.Applier, ingestQ *pipeline.IngestQueue, recordingsQ *recordingsQueue, recordings service.Recordings, commands command.Bus) (func(context.Context) error, error) {
	if !isReader(cfg) {
		bblog.Go("background-worker", func() {
			pipeline.RunWorker(ctx, cfg, eventSpool, applier, ingestQ, workerJobs(cfg, store, recordings))
		})
		return nil, nil
	}

	gate, err := newSchemaGate(store.Reader())
	if err != nil {
		return nil, err
	}
	var fallbacks []*pipeline.FallbackForwarder
	if rb, ok := commands.(*readerCommandBus); ok {
		fallbacks = append(fallbacks, rb.fallback)
	}
	if recordingsQ != nil && recordingsQ.fallback != nil {
		fallbacks = append(fallbacks, recordingsQ.fallback)
	}
	bblog.Go("reader-worker", func() {
		pipeline.RunReaderWorker(ctx, cfg, eventSpool, ingestQ, fallbacks)
	})
	return gate.Ready, nil
}

// workerJobs wires the maintenance pass and the volume-drop check into the
// worker loop.
func workerJobs(cfg config.Config, store *repository.Store, recordings service.Recordings) pipeline.Jobs {
	volumeDrop := maintenance.NewVolumeDropCheck(store)
	return pipeline.Jobs{
		Maintenance: func(ctx context.Context) {
			maintenance.Run(ctx, cfg, store, recordings)
		},
		StartupDelay:    maintenance.StartupDelay,
		VolumeDrop:      volumeDrop.Run,
		VolumeDropEvery: maintenance.VolumeDropInterval,
	}
}
