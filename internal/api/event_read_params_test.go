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
// ages, plus one other event name and one non-canonical environment, and returns a server, the project id and an
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
	// Ingest files every value under a canonical name, but a stored row can
	// still hold another one; a filter on it has to keep working.
	add("9", "cta_click", "s8", "desktop", "preview-42", time.Hour)
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
		for _, valid := range []string{"production", "staging", "test", "development", "preview-42"} {
			if !strings.Contains(w.Body.String(), valid) {
				t.Errorf("%s: the 400 should list %q among the accepted values, got %s", route, valid, w.Body.String())
			}
		}
	}
}

// matchedEvents is how many events a read answered with: the list length for
// /events, total_events for the aggregates.
func matchedEvents(t *testing.T, route string, body []byte) int {
	t.Helper()
	var resp struct {
		Events      []json.RawMessage `json:"events"`
		TotalEvents int               `json:"total_events"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	if route == "events" {
		return len(resp.Events)
	}
	return resp.TotalEvents
}

// An environment the project has recorded works even when it is not a
// canonical name. A canonical name the project has no events under yet is
// valid and counts zero, so a new project's first readout is not a 400.
func TestEventReads_RecordedAndCanonicalEnvironments(t *testing.T) {
	srv, pid, key := seedEventReads(t)
	for route, prefix := range eventReadPaths(pid) {
		for env, want := range map[string]int{"preview-42": 1, "staging": 0, "stg": 0} {
			w := withKey(t, srv, http.MethodGet, prefix+"environment="+env, key, nil)
			if w.Code != http.StatusOK {
				t.Errorf("%s environment=%s: want 200, got %d (%s)", route, env, w.Code, w.Body.String())
				continue
			}
			if got := matchedEvents(t, route, w.Body.Bytes()); got != want {
				t.Errorf("%s environment=%s: got %d events, want %d", route, env, got, want)
			}
		}
	}
}

// /events rejected a parameter it did not know (#401). event-counts and
// event-breakdown ignored one, so exclude_bot=true (missing s) counted bots and
// env=test counted every environment, with a 200. All three now answer 400 and
// name the parameter.
func TestEventReads_RejectUnknownParams(t *testing.T) {
	srv, pid, key := seedEventReads(t)
	base := "/api/v1/projects/" + pid
	for _, c := range []struct{ path, param string }{
		{base + "/events?env=test", "env"},
		{base + "/event-counts?exclude_bot=true", "exclude_bot"},
		{base + "/event-counts?env=test", "env"},
		{base + "/event-breakdown?event=cta_click&by=device_type&exclude_bot=true", "exclude_bot"},
		{base + "/event-breakdown?event=cta_click&by=device_type&name=page_view", "name"},
	} {
		w := withKey(t, srv, http.MethodGet, c.path, key, nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("GET %s: want 400, got %d (%s)", c.path, w.Code, w.Body.String())
			continue
		}
		var resp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if want := `unknown query parameter "` + c.param + `"`; !strings.Contains(resp.Error, want) {
			t.Errorf("GET %s: want %q in the error, got %q", c.path, want, resp.Error)
		}
	}
}
