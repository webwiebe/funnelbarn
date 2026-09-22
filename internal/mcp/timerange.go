package mcp

import (
	"time"
)

// defaultRange is the window a tool uses when neither from nor to is given.
const defaultRange = 7 * 24 * time.Hour

// parseRange parses the `from` / `to` tool arguments. Each accepts RFC 3339
// or YYYY-MM-DD (a date-only `to` includes that whole day, UTC). An empty `to`
// is now; an empty `from` is `to` minus seven days.
func parseRange(from, to string, now time.Time) (time.Time, time.Time, error) {
	end := now.UTC()
	if to != "" {
		t, dateOnly, err := parseTime(to)
		if err != nil {
			return time.Time{}, time.Time{}, invalidInput("to: %q is not RFC 3339 or YYYY-MM-DD", to)
		}
		if dateOnly {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		end = t
	}
	start := end.Add(-defaultRange)
	if from != "" {
		t, _, err := parseTime(from)
		if err != nil {
			return time.Time{}, time.Time{}, invalidInput("from: %q is not RFC 3339 or YYYY-MM-DD", from)
		}
		start = t
	}
	if start.After(end) {
		return time.Time{}, time.Time{}, invalidInput("from (%s) is after to (%s)", start.Format(time.RFC3339), end.Format(time.RFC3339))
	}
	return start, end, nil
}

func parseTime(s string) (t time.Time, dateOnly bool, err error) {
	if t, err = time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), false, nil
	}
	if t, err = time.Parse(time.DateOnly, s); err == nil {
		return t, true, nil
	}
	return time.Time{}, false, err
}
