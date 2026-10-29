package api

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// seedEventReads writes cta_click events across devices, environments and
// ages, plus one other event name, and returns a server, the project id and an
// analytics:read key for it.
func seedEventReads(t *testing.T) (*Server, string, string) {
	t.Helper()
	srv, store := newFlagTokenServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "Professionals", "professionals")
	now := time.Now().UTC()
	add := func(id, name, session, device, env string, age time.Duration) {
		t.Helper()
		at := now.Add(-age)
		if err := store.InsertEvent(ctx, repository.Event{
			ID: id, ProjectID: p.ID, SessionID: session, Name: name, DeviceType: device,
			Environment: env, IngestID: "ing-" + id, OccurredAt: at, CreatedAt: at,
		}); err != nil {
			t.Fatal(err)
		}
	}
	add("1", "cta_click", "s1", "desktop", "production", time.Hour)
	add("2", "cta_click", "s1", "desktop", "production", 2*time.Hour)
	add("3", "cta_click", "s2", "desktop", "production", 3*time.Hour)
	add("4", "cta_click", "s3", "mobile", "production", time.Hour)
	add("5", "cta_click", "s4", "bot", "production", time.Hour)
	add("6", "cta_click", "s5", "desktop", "test", time.Hour)
	add("7", "cta_click", "s6", "mobile", "production", 10*24*time.Hour)
	add("8", "page_view", "s7", "desktop", "production", time.Hour)
	key := scopedKey(t, store, p.ID, "readout-key", repository.APIKeyScopeAnalyticsRead)
	return srv, p.ID, key
}

// The acceptance line of #400: "cta_click by device_type for the last 7 days
// in production, bots excluded" is one request from a script with an
// analytics:read key. The test-environment click, the bot, the 10-day-old
// click and the page_view all stay out.
func TestEventBreakdown_AcceptanceRequestWithToken(t *testing.T) {
	srv, pid, key := seedEventReads(t)
	path := "/api/v1/projects/" + pid + "/event-breakdown?event=cta_click&by=device_type&range=7d&environment=production&exclude_bots=true"
	w := withKey(t, srv, http.MethodGet, path, key, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", w.Code, w.Body.String())
	}
	var resp struct {
		Values      []repository.BreakdownRow `json:"values"`
		TotalEvents int64                     `json:"total_events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	want := []repository.BreakdownRow{{Value: "desktop", Count: 3, Sessions: 2}, {Value: "mobile", Count: 1, Sessions: 1}}
	if !reflect.DeepEqual(resp.Values, want) {
		t.Errorf("values: got %+v, want %+v", resp.Values, want)
	}
	if resp.TotalEvents != 4 {
		t.Errorf("total_events: got %d, want 4", resp.TotalEvents)
	}
}

// eventReadPaths are the three token-readable event reads that take an
// environment filter, each with the query it needs besides environment. The
// window is explicit: the default one ends at the request time, which the
// responses echo, so two requests a second apart would differ.
func eventReadPaths(pid string) map[string]string {
	base := "/api/v1/projects/" + pid
	to := time.Now().UTC().Add(time.Minute).Truncate(time.Second)
	window := "from=" + to.AddDate(0, 0, -30).Format(time.RFC3339) + "&to=" + to.Format(time.RFC3339) + "&"
	return map[string]string{
		"events":          base + "/events?name=cta_click&" + window,
		"event-counts":    base + "/event-counts?" + window,
		"event-breakdown": base + "/event-breakdown?event=cta_click&by=device_type&" + window,
	}
}

// Ingest stores "prod" as "production", and funnel analysis normalises its
// environment argument the same way (repository.EnvironmentRule). The event
// reads compared the raw string, so environment=prod returned nothing.
func TestEventReads_EnvironmentAliasMatchesCanonical(t *testing.T) {
	srv, pid, key := seedEventReads(t)
	for route, prefix := range eventReadPaths(pid) {
		canonical := withKey(t, srv, http.MethodGet, prefix+"environment=production", key, nil)
		alias := withKey(t, srv, http.MethodGet, prefix+"environment=prod", key, nil)
		if canonical.Code != http.StatusOK || alias.Code != http.StatusOK {
			t.Errorf("%s: want 200 for both, got production=%d prod=%d (%s)", route, canonical.Code, alias.Code, alias.Body.String())
			continue
		}
		if canonical.Body.String() != alias.Body.String() {
			t.Errorf("%s: environment=prod differs from environment=production\n prod:       %s\n production: %s",
				route, alias.Body.String(), canonical.Body.String())
		}
	}
}

// environment.Normalize files an unknown value under production, which suits
// deployment config. On a read filter that turns a typo into production
// numbers, so an unknown value is a 400 that names the accepted values.
func TestEventReads_RejectUnknownEnvironment(t *testing.T) {
	srv, pid, key := seedEventReads(t)
	for route, prefix := range eventReadPaths(pid) {
		w := withKey(t, srv, http.MethodGet, prefix+"environment=prodution", key, nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400 for environment=prodution, got %d (%s)", route, w.Code, w.Body.String())
			continue
		}
		if !strings.Contains(w.Body.String(), "production") {
			t.Errorf("%s: the 400 should list the accepted values, got %s", route, w.Body.String())
		}
	}
}

// /events rejects a parameter it does not know (#401). event-counts and
// event-breakdown ignored one, so exclude_bot=true (missing s) counted bots and
// env=test counted every environment, with a 200.
func TestEventCountsAndBreakdown_RejectUnknownParams(t *testing.T) {
	srv, pid, key := seedEventReads(t)
	base := "/api/v1/projects/" + pid
	for _, path := range []string{
		base + "/event-counts?exclude_bot=true",
		base + "/event-counts?env=test",
		base + "/event-breakdown?event=cta_click&by=device_type&exclude_bot=true",
		base + "/event-breakdown?event=cta_click&by=device_type&name=page_view",
	} {
		w := withKey(t, srv, http.MethodGet, path, key, nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("GET %s: want 400, got %d (%s)", path, w.Code, w.Body.String())
			continue
		}
		if !strings.Contains(w.Body.String(), "unknown query parameter") {
			t.Errorf("GET %s: want an unknown-parameter message, got %s", path, w.Body.String())
		}
	}
}
