// Package timerange provides a single, canonical time-range resolver for HTTP
// query parameters. It eliminates the duplicated parsing block that previously
// appeared in dashboard, abtests, and funnels handlers.
package timerange

import (
	"net/url"
	"time"
)

// Range is a resolved [From, To) window, both in UTC.
type Range struct {
	From time.Time
	To   time.Time
}

// Parse resolves a time range from URL query parameters.
//
// Supported shorthand via ?range=:
//   - "24h" → last 24 hours
//   - "7d"  → last 7 days
//   - "30d" → last 30 days (default when range param is absent or unrecognised)
//
// Explicit RFC 3339 overrides via ?from= and ?to= take precedence over the
// shorthand. If a parse fails, the shorthand value is kept.
func Parse(q url.Values) Range {
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -30)

	switch q.Get("range") {
	case "24h":
		from = to.Add(-24 * time.Hour)
	case "7d":
		from = to.AddDate(0, 0, -7)
	case "30d":
		from = to.AddDate(0, 0, -30)
	}

	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}

	return Range{From: from, To: to}
}
