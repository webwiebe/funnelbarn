package timerange_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/timerange"
)

func TestParse_Default(t *testing.T) {
	r := timerange.Parse(url.Values{})
	now := time.Now().UTC()
	// Default: last 30 days.
	if r.To.After(now.Add(time.Second)) {
		t.Errorf("To should be approximately now")
	}
	want := now.AddDate(0, 0, -30)
	if r.From.After(want.Add(time.Second)) || want.After(r.From.Add(time.Second)) {
		t.Errorf("From should be ~30 days ago, got %v (want ~%v)", r.From, want)
	}
}

func TestParse_Shorthands(t *testing.T) {
	cases := []struct {
		shorthand  string
		minusDays  int
		minusHours int
	}{
		{"7d", 7, 0},
		{"30d", 30, 0},
	}
	for _, tc := range cases {
		t.Run(tc.shorthand, func(t *testing.T) {
			q := url.Values{"range": []string{tc.shorthand}}
			r := timerange.Parse(q)
			expectedFrom := time.Now().UTC().AddDate(0, 0, -tc.minusDays)
			diff := expectedFrom.Sub(r.From)
			if diff < 0 {
				diff = -diff
			}
			if diff > 2*time.Second {
				t.Errorf("From off by %v for shorthand %q", diff, tc.shorthand)
			}
		})
	}
}

func TestParse_24h(t *testing.T) {
	q := url.Values{"range": []string{"24h"}}
	r := timerange.Parse(q)
	expected := time.Now().UTC().Add(-24 * time.Hour)
	diff := expected.Sub(r.From)
	if diff < 0 {
		diff = -diff
	}
	if diff > 2*time.Second {
		t.Errorf("24h: From off by %v", diff)
	}
}

func TestParse_ExplicitOverride(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	q := url.Values{
		"from":  []string{from.Format(time.RFC3339)},
		"to":    []string{to.Format(time.RFC3339)},
		"range": []string{"7d"}, // shorthand should be overridden
	}
	r := timerange.Parse(q)
	if !r.From.Equal(from) {
		t.Errorf("From: want %v, got %v", from, r.From)
	}
	if !r.To.Equal(to) {
		t.Errorf("To: want %v, got %v", to, r.To)
	}
}

func TestParse_InvalidDateIgnored(t *testing.T) {
	q := url.Values{"from": []string{"not-a-date"}}
	r := timerange.Parse(q)
	// Should fall back to default (30 days ago).
	expected := time.Now().UTC().AddDate(0, 0, -30)
	diff := expected.Sub(r.From)
	if diff < 0 {
		diff = -diff
	}
	if diff > 2*time.Second {
		t.Errorf("invalid date not ignored: From = %v", r.From)
	}
}
