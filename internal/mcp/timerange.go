package mcp

import (
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/timerange"
)

// parseRange parses the `from` / `to` tool arguments with timerange.ParseDates
// (RFC 3339 or YYYY-MM-DD, default the last seven days) and reports a bad
// value as invalid input.
func parseRange(from, to string, now time.Time) (time.Time, time.Time, error) {
	r, err := timerange.ParseDates(from, to, now)
	if err != nil {
		return time.Time{}, time.Time{}, invalidInput("%s", err.Error())
	}
	return r.From, r.To, nil
}
