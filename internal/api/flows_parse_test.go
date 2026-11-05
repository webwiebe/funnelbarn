package api

import (
	"net/url"
	"testing"
	"time"
)

func TestParseFlowsRange(t *testing.T) {
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
		from, to := parseFlowsRange(q, now)
		if !from.Equal(tc.from) || !to.Equal(tc.to) {
			t.Errorf("parseFlowsRange(%q) = (%v, %v), want (%v, %v)", tc.query, from, to, tc.from, tc.to)
		}
	}
}

func TestParseFlowsDepth(t *testing.T) {
	cases := map[string]int{
		"":    5,
		"1":   1,
		"10":  10,
		"0":   5,
		"11":  5,
		"3x":  3,
		"1a0": 10,
		"-4":  4,
		"abc": 5,
	}
	for in, want := range cases {
		if got := parseFlowsDepth(in); got != want {
			t.Errorf("parseFlowsDepth(%q) = %d, want %d", in, got, want)
		}
	}
}
