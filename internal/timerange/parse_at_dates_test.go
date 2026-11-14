package timerange_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/timerange"
)

func TestParseAt(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	at := func(s string) time.Time {
		t.Helper()
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	cases := []struct {
		query    string
		from, to time.Time
	}{
		{query: "", from: now.AddDate(0, 0, -30), to: now},
		{query: "range=24h", from: now.Add(-24 * time.Hour), to: now},
		{query: "range=7d", from: now.AddDate(0, 0, -7), to: now},
		{query: "range=30d", from: now.AddDate(0, 0, -30), to: now},
		{query: "range=1y", from: now.AddDate(0, 0, -30), to: now},
		{query: "range=7d&from=2026-10-01T00:00:00Z", from: at("2026-10-01T00:00:00Z"), to: now},
		{query: "from=2026-10-01T00:00:00Z&to=2026-10-02T00:00:00Z", from: at("2026-10-01T00:00:00Z"), to: at("2026-10-02T00:00:00Z")},
		{query: "from=yesterday&to=2026-13-01", from: now.AddDate(0, 0, -30), to: now},
	}
	for _, tc := range cases {
		q, err := url.ParseQuery(tc.query)
		if err != nil {
			t.Fatal(err)
		}
		r := timerange.ParseAt(q, now)
		if !r.From.Equal(tc.from) || !r.To.Equal(tc.to) {
			t.Errorf("ParseAt(%q) = (%v, %v), want (%v, %v)", tc.query, r.From, r.To, tc.from, tc.to)
		}
	}
}

func TestParseDates(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name, from, to   string
		wantFrom, wantTo time.Time
		wantErr          string
	}{
		{name: "default seven days", wantFrom: now.Add(-7 * 24 * time.Hour), wantTo: now},
		{name: "dates", from: "2026-09-01", to: "2026-09-02",
			wantFrom: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			wantTo:   time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond)},
		{name: "rfc3339", from: "2026-09-01T10:00:00Z", to: "2026-09-01T14:00:00+02:00",
			wantFrom: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
			wantTo:   time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)},
		{name: "only to", to: "2026-09-10T00:00:00Z",
			wantFrom: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC),
			wantTo:   time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)},
		{name: "bad from", from: "yesterday", wantErr: `from: "yesterday" is not RFC 3339 or YYYY-MM-DD`},
		{name: "bad to", to: "09/01/2026", wantErr: `to: "09/01/2026" is not RFC 3339 or YYYY-MM-DD`},
		{name: "reversed", from: "2026-09-05", to: "2026-09-01",
			wantErr: "from (2026-09-05T00:00:00Z) is after to (2026-09-01T23:59:59Z)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := timerange.ParseDates(tc.from, tc.to, now)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDates: %v", err)
			}
			if !r.From.Equal(tc.wantFrom) || !r.To.Equal(tc.wantTo) {
				t.Errorf("got %s..%s, want %s..%s", r.From, r.To, tc.wantFrom, tc.wantTo)
			}
		})
	}
}
