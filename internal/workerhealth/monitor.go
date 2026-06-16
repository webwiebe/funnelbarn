// Package workerhealth detects silent failure modes in the ingest worker so they
// surface instead of going unnoticed. Its methods are pure state machines that
// report when something looks wrong; the caller decides how to react (typically a
// slog.Error, which the bblog handler forwards to BugBarn as an issue).
//
// It covers two failure modes that previously produced no signal at all:
//   - the spool consumer stalling (cursor frozen while a backlog accumulates),
//     which once halted all ingestion for ~5 days unnoticed;
//   - geo enrichment being enabled but resolving nothing (e.g. the real client
//     IP never reaching the service), which left country_code empty for every
//     event ever recorded.
package workerhealth

import "time"

// Monitor tracks ingest-worker progress and geo-lookup outcomes. It is not safe
// for concurrent use; the ingest worker drives it from a single goroutine.
type Monitor struct {
	stallAfter    time.Duration
	geoSampleSize int
	geoReAlert    time.Duration
	now           func() time.Time

	started    bool
	lastOffset int64
	progressAt time.Time
	stalled    bool

	geoAttempts  int
	geoHits      int
	lastGeoAlert time.Time
}

// Options configures a Monitor. Zero values fall back to sensible defaults.
type Options struct {
	StallAfter    time.Duration // offset may stay frozen with pending data this long before alerting (default 5m)
	GeoSampleSize int           // lookups per geo evaluation window (default 200)
	GeoReAlert    time.Duration // minimum gap between geo alerts (default 6h)
	Now           func() time.Time
}

// New returns a Monitor with defaults applied for any unset option.
func New(o Options) *Monitor {
	if o.StallAfter <= 0 {
		o.StallAfter = 5 * time.Minute
	}
	if o.GeoSampleSize <= 0 {
		o.GeoSampleSize = 200
	}
	if o.GeoReAlert <= 0 {
		o.GeoReAlert = 6 * time.Hour
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Monitor{
		stallAfter:    o.StallAfter,
		geoSampleSize: o.GeoSampleSize,
		geoReAlert:    o.GeoReAlert,
		now:           o.Now,
	}
}

// CheckProgress should be called once per worker tick with the current cursor
// offset and the number of pending (unconsumed) bytes in the active spool file.
// It reports a stall when there is pending data but the offset has not advanced
// within StallAfter. It fires at most once per stall — it stays quiet until the
// offset advances again, then re-arms. stalledFor is how long the offset had been
// frozen when the alert fires.
func (m *Monitor) CheckProgress(offset, pending int64) (alert bool, stalledFor time.Duration) {
	t := m.now()

	if !m.started {
		m.started = true
		m.lastOffset = offset
		m.progressAt = t
		return false, 0
	}

	if offset != m.lastOffset {
		// Progress — the consumer is draining. Re-arm.
		m.lastOffset = offset
		m.progressAt = t
		m.stalled = false
		return false, 0
	}

	if pending <= 0 {
		// Caught up / idle (offset == end of file). Not a stall; keep the timer
		// fresh so an idle period doesn't count toward a future stall window.
		m.progressAt = t
		return false, 0
	}

	if m.stalled {
		return false, 0 // already alerted for this stall
	}

	if d := t.Sub(m.progressAt); d >= m.stallAfter {
		m.stalled = true
		return true, d
	}
	return false, 0
}

// Stalled reports whether the consumer is currently in a confirmed-stall state
// (set true when CheckProgress alerts, cleared when the offset next advances).
func (m *Monitor) Stalled() bool { return m.stalled }

// RecordGeo records the outcome of one geo lookup (hit = a country was resolved).
// When a full window of GeoSampleSize lookups resolves zero countries it returns
// true once, throttled by GeoReAlert, so a broken client-IP/proxy chain (geo
// enabled but never matching) is surfaced. attempts is the window size that
// produced zero hits.
func (m *Monitor) RecordGeo(hit bool) (alert bool, attempts int) {
	m.geoAttempts++
	if hit {
		m.geoHits++
	}
	if m.geoAttempts < m.geoSampleSize {
		return false, 0
	}

	hits := m.geoHits
	attempts = m.geoAttempts
	m.geoAttempts = 0
	m.geoHits = 0

	if hits > 0 {
		return false, 0
	}

	t := m.now()
	if m.lastGeoAlert.IsZero() || t.Sub(m.lastGeoAlert) >= m.geoReAlert {
		m.lastGeoAlert = t
		return true, attempts
	}
	return false, 0
}
