package command

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// SetOutageClock gives b's publish and receive outage tracking a test clock.
func SetOutageClock(b *RedisBus, now func() time.Time) {
	b.publishOutage.Now = now
	b.receiveOutage.Now = now
}

// AppliedCount reads funnelbarn_command_applied_total for one label pair from
// the default registry.
func AppliedCount(kind, result string) float64 {
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return -1
	}
	for _, mf := range mfs {
		if mf.GetName() != "funnelbarn_command_applied_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["kind"] == kind && labels["result"] == result {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}
