package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func seedBreakdown(t *testing.T) (*Server, string) {
	t.Helper()
	srv, store := newTestServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "Breakdown", "breakdown")
	at := time.Now().UTC().Add(-time.Hour)
	add := func(id, session, device, ref, props string) {
		t.Helper()
		if err := store.InsertEvent(ctx, repository.Event{
			ID: id, ProjectID: p.ID, SessionID: session, Name: "signup", DeviceType: device,
			ReferrerDomain: ref, Properties: props, IngestID: "ing-" + id, OccurredAt: at, CreatedAt: at,
		}); err != nil {
			t.Fatal(err)
		}
	}
	add("1", "s1", "desktop", "google.com", `{"plan":"pro"}`)
	add("2", "s1", "desktop", "google.com", `{"plan":"free"}`)
	add("3", "s2", "mobile", "", `{"plan":"pro"}`)
	add("4", "s3", "bot", "spam.example", `{"plan":"pro"}`)
	add("5", "s4", "desktop", "bing.com", `{"real_signup":true,"seats":3}`)
	add("6", "s5", "mobile", "bing.com", `{"real_signup":false}`)
	add("7", "s6", "mobile", "bing.com", `{"real_signup":"true"}`)
	return srv, p.ID
}

func TestHandleEventBreakdown(t *testing.T) {
	srv, pid := seedBreakdown(t)
	get := func(q url.Values) []repository.BreakdownRow {
		t.Helper()
		q.Set("event", "signup")
		w := getJSON(t, srv, "/api/v1/projects/"+pid+"/event-breakdown?"+q.Encode(), nil)
		if w.Code != http.StatusOK {
			t.Fatalf("?%s: want 200, got %d (%s)", q.Encode(), w.Code, w.Body.String())
		}
		var resp struct {
			Values []repository.BreakdownRow `json:"values"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		return resp.Values
	}
	cases := []struct {
		name string
		q    url.Values
		want []repository.BreakdownRow
	}{
		{"device", url.Values{"by": {"device_type"}}, []repository.BreakdownRow{
			{Value: "desktop", Count: 3, Sessions: 2}, {Value: "mobile", Count: 3, Sessions: 3}, {Value: "bot", Count: 1, Sessions: 1}}},
		{"no bots", url.Values{"by": {"device_type"}, "exclude_bots": {"true"}}, []repository.BreakdownRow{
			{Value: "desktop", Count: 3, Sessions: 2}, {Value: "mobile", Count: 3, Sessions: 3}}},
		{"property", url.Values{"by": {"referrer_domain"}, "property": {"plan"}, "value": {"pro"}}, []repository.BreakdownRow{
			{Value: "", Count: 1, Sessions: 1}, {Value: "google.com", Count: 1, Sessions: 1}, {Value: "spam.example", Count: 1, Sessions: 1}}},
		{"boolean property", url.Values{"by": {"device_type"}, "property": {"real_signup"}, "value": {"true"}}, []repository.BreakdownRow{
			{Value: "desktop", Count: 1, Sessions: 1}, {Value: "mobile", Count: 1, Sessions: 1}}},
	}
	for _, c := range cases {
		if got := get(c.q); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestHandleEventBreakdown_RejectsBadParams(t *testing.T) {
	srv, pid := seedBreakdown(t)
	for _, q := range []string{
		"by=device_type",          // no event
		"event=signup",            // no by
		"event=signup&by=country", // unsupported dimension
		"event=signup&by=utm_source&exclude_bots=maybe", // bad bool
		"event=signup&by=utm_source&property=plan",      // property without value
		"event=signup&by=utm_source&value=pro",          // value without property
		"event=signup&by=utm_source&property=a.b&value=x",
		"event=signup&by=utm_source&from=yesterday",
	} {
		w := getJSON(t, srv, "/api/v1/projects/"+pid+"/event-breakdown?"+q, nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("?%s: want 400, got %d", q, w.Code)
		}
	}
}

func TestHandleEventCounts_ExcludeBotsAndProperty(t *testing.T) {
	srv, pid := seedBreakdown(t)
	count := func(q string) int64 {
		t.Helper()
		w := getJSON(t, srv, "/api/v1/projects/"+pid+"/event-counts?"+q, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("?%s: want 200, got %d (%s)", q, w.Code, w.Body.String())
		}
		var resp struct {
			Total int64 `json:"total_events"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		return resp.Total
	}
	for q, want := range map[string]int64{
		"range=7d":                                           7,
		"range=7d&exclude_bots=true":                         6,
		"range=7d&property=plan&value=pro":                   3,
		"range=7d&property=plan&value=pro&exclude_bots=true": 2,
		"range=7d&property=real_signup&value=true":           2, // JSON true and the string "true"
		"range=7d&property=real_signup&value=false":          1,
		"range=7d&property=seats&value=3":                    1,
		"range=7d&property=plan&value=true":                  0,
	} {
		if got := count(q); got != want {
			t.Errorf("?%s: got %d, want %d", q, got, want)
		}
	}
}
