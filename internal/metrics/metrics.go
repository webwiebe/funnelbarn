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

	// EventErrors is labeled by "reason": "retry" for a processing failure
	// that will be retried, "dead_letter" for one that exhausted retries and
	// was written to the dead-letter spool. All increment call sites must
	// supply this label (CounterVec requires it on every observation).
	EventErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "funnelbarn_event_errors_total",
		Help: "Total events that failed processing, labeled by reason (retry vs dead_letter).",
	}, []string{"reason"})

	EventProcessingDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "funnelbarn_event_processing_duration_seconds",
		Help:    "Time to process and persist a single event in the background worker.",
		Buckets: prometheus.DefBuckets,
	})

	EventsPurged = promauto.NewCounter(prometheus.CounterOpts{
		Name: "funnelbarn_events_purged_total",
		Help: "Total old events deleted by the retention purge job.",
	})

	SpoolQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "funnelbarn_spool_queue_depth",
		Help: "Approximate number of unprocessed event records in the spool.",
	})

	// Worker health signals (mirror the workerhealth detectors so they're
	// graphable trends in SpanBarn, not just discrete BugBarn issues).
	IngestPendingBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "funnelbarn_ingest_pending_bytes",
		Help: "Unconsumed bytes in the active spool file (consumer backlog).",
	})

	IngestStalled = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "funnelbarn_ingest_stalled",
		Help: "1 when the ingest consumer is stalled (backlog not draining), else 0.",
	})

	GeoLookups = promauto.NewCounter(prometheus.CounterOpts{
		Name: "funnelbarn_geo_lookups_total",
		Help: "Total geo lookups attempted by the worker.",
	})

	GeoHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "funnelbarn_geo_hits_total",
		Help: "Total geo lookups that resolved a country.",
	})

	// External call instrumentation (R2 storage, IAMBarn, OIDC).
	R2Requests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "funnelbarn_r2_requests_total",
		Help: "Total R2 storage requests by operation and outcome.",
	}, []string{"operation", "outcome"})

	R2RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "funnelbarn_r2_request_duration_seconds",
		Help:    "R2 storage request latency by operation.",
		Buckets: prometheus.DefBuckets,
	}, []string{"operation"})

	IAMBarnRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "funnelbarn_iambarn_requests_total",
		Help: "Total IAMBarn requests by operation and outcome.",
	}, []string{"operation", "outcome"})

	IAMBarnRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "funnelbarn_iambarn_request_duration_seconds",
		Help:    "IAMBarn request latency by operation.",
		Buckets: prometheus.DefBuckets,
	}, []string{"operation"})

	OIDCRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "funnelbarn_oidc_requests_total",
		Help: "Total OIDC requests by operation and outcome.",
	}, []string{"operation", "outcome"})

	OIDCRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "funnelbarn_oidc_request_duration_seconds",
		Help:    "OIDC request latency by operation.",
		Buckets: prometheus.DefBuckets,
	}, []string{"operation"})

	// Business-level: flag evaluation (dashboard playground + SDK evaluate endpoint).
	// Labelled by "reason" only (DISABLED/TARGETING_MATCH/SPLIT/default/ERROR) — a
	// small fixed set — never by project or flag id, which are unbounded.
	FlagEvaluations = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "funnelbarn_flag_evaluations_total",
		Help: "Total flag evaluations by outcome reason.",
	}, []string{"reason"})

	FlagEvaluationDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "funnelbarn_flag_evaluation_duration_seconds",
		Help:    "Duration of flag evaluation (assignment) calls.",
		Buckets: prometheus.DefBuckets,
	})

	// Business-level: funnel analysis.
	FunnelAnalysisDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "funnelbarn_funnel_analysis_duration_seconds",
		Help:    "Duration of funnel analysis computation.",
		Buckets: prometheus.DefBuckets,
	})
)
