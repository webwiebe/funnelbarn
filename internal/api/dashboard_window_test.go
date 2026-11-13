package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestDashboardWindow(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		wantSpan   time.Duration // to - from, 0 when explicit
		wantHourly bool
		wantFrom   string
		wantTo     string
	}{
		{name: "default is 30 days daily", query: "", wantSpan: 30 * 24 * time.Hour},
		{name: "24h is hourly", query: "range=24h", wantSpan: 24 * time.Hour, wantHourly: true},
		{name: "7d is hourly", query: "range=7d", wantSpan: 7 * 24 * time.Hour, wantHourly: true},
		{name: "30d is daily", query: "range=30d", wantSpan: 30 * 24 * time.Hour},
		{name: "unknown range keeps 30 days daily", query: "range=1y", wantSpan: 30 * 24 * time.Hour},
		{name: "explicit from and to override the range", query: "range=24h&from=2026-01-01T00:00:00Z&to=2026-01-05T00:00:00Z", wantHourly: true,
			wantFrom: "2026-01-01T00:00:00Z", wantTo: "2026-01-05T00:00:00Z"},
		{name: "unparseable from is ignored", query: "from=yesterday", wantSpan: 30 * 24 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/v1/projects/p/dashboard?"+tc.query, nil)
			from, to, hourly := dashboardWindow(r)
			if hourly != tc.wantHourly {
				t.Errorf("hourly = %v, want %v", hourly, tc.wantHourly)
			}
			if tc.wantFrom != "" {
				if got := from.Format(time.RFC3339); got != tc.wantFrom {
					t.Errorf("from = %s, want %s", got, tc.wantFrom)
				}
				if got := to.Format(time.RFC3339); got != tc.wantTo {
					t.Errorf("to = %s, want %s", got, tc.wantTo)
				}
				return
			}
			span := to.Sub(from)
			// 30 days via AddDate can differ from 720h only across DST, and the
			// window is in UTC, so the spans are exact.
			if span != tc.wantSpan {
				t.Errorf("span = %v, want %v", span, tc.wantSpan)
			}
		})
	}
}
