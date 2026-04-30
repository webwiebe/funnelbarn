package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP layer
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "funnelbarn_http_requests_total",
		Help: "Total HTTP requests by method, path, and status code.",
	}, []string{"method", "path", "status"})

	HTTPLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "funnelbarn_http_request_duration_seconds",
		Help:    "HTTP request latency.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	// Ingest pipeline
	EventsIngested = promauto.NewCounter(prometheus.CounterOpts{
		Name: "funnelbarn_events_ingested_total",
		Help: "Total events accepted into the ingest queue.",
	})

	IngestQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "funnelbarn_ingest_queue_depth",
		Help: "Current number of events waiting in the in-memory ingest queue.",
	})

	// Worker pipeline
	EventsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "funnelbarn_events_processed_total",
		Help: "Total events successfully processed by the background worker.",
	})

	EventErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "funnelbarn_event_errors_total",
		Help: "Total events that failed processing (including dead-lettered).",
	})

	EventsPurged = promauto.NewCounter(prometheus.CounterOpts{
		Name: "funnelbarn_events_purged_total",
		Help: "Total old events deleted by the retention purge job.",
	})

	SpoolQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "funnelbarn_spool_queue_depth",
		Help: "Approximate number of unprocessed event records in the spool.",
	})
)
