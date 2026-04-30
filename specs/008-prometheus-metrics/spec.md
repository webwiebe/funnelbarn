# Spec 008: Prometheus Metrics

## Goal
Add a `/metrics` endpoint serving Prometheus-format metrics so the operator has visibility into request rates, error rates, ingest queue depth, and worker throughput.

## Dependency
Add to go.mod:
```
github.com/prometheus/client_golang v1.19.1
```
Run: `go get github.com/prometheus/client_golang@v1.19.1`

## Files to modify / create
- `internal/metrics/metrics.go` (new) — metric definitions and registration
- `internal/api/middleware.go` — update `requestLogger` to record HTTP metrics
- `internal/api/server.go` — add `/metrics` route (no auth required)
- `internal/ingest/handler.go` — record ingest queue depth and accepted event count
- `cmd/funnelbarn/main.go` — record worker metrics (processed, errors, purged)

## Metric definitions (internal/metrics/metrics.go)

```go
package metrics

import "github.com/prometheus/client_golang/prometheus"
import "github.com/prometheus/client_golang/prometheus/promauto"

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
)
```

## Wiring

### internal/api/middleware.go — update requestLogger
After the handler returns, record metrics:
```go
import "github.com/wiebe-xyz/funnelbarn/internal/metrics"

// Inside requestLogger, after next.ServeHTTP:
statusStr := strconv.Itoa(status)
metrics.HTTPRequests.WithLabelValues(r.Method, r.URL.Path, statusStr).Inc()
metrics.HTTPLatency.WithLabelValues(r.Method, r.URL.Path).Observe(time.Since(start).Seconds())
```
Add `"strconv"` to imports.

### internal/api/server.go — add /metrics route
At the top of registerRoutes(), before all other routes:
```go
import "github.com/prometheus/client_golang/prometheus/promhttp"

s.mux.Handle("GET /metrics", promhttp.Handler())
```

### internal/ingest/handler.go — record ingest metrics
In `ServeHTTP`, after successfully enqueuing the event (the `s.queue <- record` line):
```go
import "github.com/wiebe-xyz/funnelbarn/internal/metrics"
metrics.EventsIngested.Inc()
```

After reading queue length (add a goroutine or update inline):
In `Start()`, in the batch-flush loop, record queue depth periodically. After each drain iteration:
```go
metrics.IngestQueueDepth.Set(float64(len(s.queue)))
```

### cmd/funnelbarn/main.go — record worker metrics
In `runBackgroundWorker()`:

After `worker.PersistEvent(ctx, store, event)` succeeds:
```go
metrics.EventsProcessed.Inc()
```

After dead-lettering (on max retries or persistent error):
```go
metrics.EventErrors.Inc()
```

After `store.PurgeOldEvents(ctx, cutoff)` returns n rows deleted:
```go
metrics.EventsPurged.Add(float64(n))
```

Import: `"github.com/wiebe-xyz/funnelbarn/internal/metrics"`

## Acceptance criteria
- `GET /metrics` returns 200 with `text/plain; version=0.0.4` content type (Prometheus exposition format)
- `funnelbarn_http_requests_total`, `funnelbarn_events_ingested_total`, `funnelbarn_events_processed_total` appear in the output
- `go build ./...` passes
- `/metrics` endpoint is NOT behind `requireSession` — Prometheus scrapes it without auth
