package command

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	queueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "funnelbarn_command_queue_depth",
		Help: "Commands waiting in the dispatcher buffer.",
	})

	applied = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "funnelbarn_command_applied_total",
		Help: "Commands applied by the dispatcher, labeled by kind and result (ok or error).",
	}, []string{"kind", "result"})

	submitWait = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "funnelbarn_command_submit_wait_seconds",
		Help:    "Time a submitter waited to enqueue a command. Non-zero means the buffer was full.",
		Buckets: []float64{0.0001, 0.001, 0.01, 0.1, 0.5, 1, 2.5, 5, 10, 30},
	})
)
