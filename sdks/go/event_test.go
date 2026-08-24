package funnelbarn

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// bodyCollector spins up an ingest stub and returns the channel decoded bodies
// land on. Mirrors the shape used in env_test.go.
func bodyCollector(t *testing.T, n int) (chan map[string]any, *httptest.Server) {
	t.Helper()
	bodies := make(chan map[string]any, n)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		bodies <- b
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	return bodies, srv
}

func recv(t *testing.T, bodies chan map[string]any) map[string]any {
	t.Helper()
	select {
	case b := <-bodies:
		return b
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for an event")
		return nil
	}
}

// The whole point of Event: every field the payload carries must be reachable.
// Before this, session_id / user_id / utm_* were marshalled but unsettable, so
// a server-sent event could never join a funnel (funnels group on session_id).
func TestTrackEvent_CarriesEveryField(t *testing.T) {
	bodies, srv := bodyCollector(t, 1)
	Init(Options{APIKey: "k", Endpoint: srv.URL, ProjectName: "p"})

	when := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	TrackEvent(Event{
		Name:        "outreach_opened",
		URL:         "https://example.com/landing",
		Referrer:    "https://mail.example.com/",
		UTMSource:   "outreach",
		UTMMedium:   "email",
		UTMCampaign: "nl-intro",
		UTMTerm:     "intro-1",
		UTMContent:  "hero-cta",
		Properties:  map[string]any{"step": "intro-1"},
		UserID:      "lead-77607",
		SessionID:   "send-abc123",
		Timestamp:   when,
	})
	if err := Shutdown(3 * time.Second); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	b := recv(t, bodies)
	for _, tc := range []struct{ key, want string }{
		{"name", "outreach_opened"},
		{"url", "https://example.com/landing"},
		{"referrer", "https://mail.example.com/"},
		{"utm_source", "outreach"},
		{"utm_medium", "email"},
		{"utm_campaign", "nl-intro"},
		{"utm_term", "intro-1"},
		{"utm_content", "hero-cta"},
		{"user_id", "lead-77607"},
		{"session_id", "send-abc123"},
	} {
		if got := b[tc.key]; got != tc.want {
			t.Errorf("%s = %v, want %q", tc.key, got, tc.want)
		}
	}
	props, ok := b["properties"].(map[string]any)
	if !ok || props["step"] != "intro-1" {
		t.Errorf("properties = %v, want step=intro-1", b["properties"])
	}
	// An explicit timestamp must survive: an event replayed from a spool is
	// about when it happened, not when we got around to sending it.
	ts, err := time.Parse(time.RFC3339Nano, b["timestamp"].(string))
	if err != nil {
		t.Fatalf("parse timestamp: %v", err)
	}
	if !ts.Equal(when) {
		t.Errorf("timestamp = %s, want %s", ts, when)
	}
}

func TestTrackEvent_ZeroTimestampMeansNow(t *testing.T) {
	bodies, srv := bodyCollector(t, 1)
	Init(Options{APIKey: "k", Endpoint: srv.URL, ProjectName: "p"})

	before := time.Now().UTC().Add(-time.Second)
	TrackEvent(Event{Name: "ping"})
	if err := Shutdown(3 * time.Second); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	ts, err := time.Parse(time.RFC3339Nano, recv(t, bodies)["timestamp"].(string))
	if err != nil {
		t.Fatalf("parse timestamp: %v", err)
	}
	if ts.Before(before) || ts.After(time.Now().UTC().Add(time.Second)) {
		t.Errorf("timestamp = %s, want ~now", ts)
	}
}

// Track and Page are now wrappers. They must be indistinguishable on the wire
// from what they sent before, or this is a breaking change wearing a bow tie.
func TestTrackAndPage_UnchangedOnTheWire(t *testing.T) {
	bodies, srv := bodyCollector(t, 2)
	Init(Options{APIKey: "k", Endpoint: srv.URL, ProjectName: "p"})

	Track("signup_completed", map[string]any{"plan": "pro"})
	Page("https://example.com/pricing", "https://google.com/")
	if err := Shutdown(3 * time.Second); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	got := map[string]map[string]any{}
	for i := 0; i < 2; i++ {
		b := recv(t, bodies)
		got[b["name"].(string)] = b
	}

	tr, ok := got["signup_completed"]
	if !ok {
		t.Fatal("no signup_completed event")
	}
	if props := tr["properties"].(map[string]any); props["plan"] != "pro" {
		t.Errorf("Track properties = %v", tr["properties"])
	}
	// Track never set these, and must still not.
	for _, k := range []string{"url", "referrer", "user_id", "session_id", "utm_source"} {
		if _, present := tr[k]; present {
			t.Errorf("Track set %s = %v, want it omitted", k, tr[k])
		}
	}

	pg, ok := got["page_view"]
	if !ok {
		t.Fatal("no page_view event")
	}
	if pg["url"] != "https://example.com/pricing" || pg["referrer"] != "https://google.com/" {
		t.Errorf("Page url/referrer = %v / %v", pg["url"], pg["referrer"])
	}
	if _, present := pg["properties"]; present {
		t.Errorf("Page set properties = %v, want omitted", pg["properties"])
	}
}

func TestTrackEvent_RefusesEmptyName(t *testing.T) {
	_, srv := bodyCollector(t, 1)
	Init(Options{APIKey: "k", Endpoint: srv.URL, ProjectName: "p"})
	defer func() { _ = Shutdown(time.Second) }()

	if TrackEvent(Event{Name: ""}) {
		t.Error("TrackEvent with an empty Name returned true, want false")
	}
}

// "Not initialised" means analytics was never switched on. That is not a lost
// event and must not be counted as one, or the drop counter alarms on every
// process that simply has no API key.
func TestTrackEvent_UninitialisedIsNotADrop(t *testing.T) {
	if err := Shutdown(time.Second); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	before := Dropped()
	if TrackEvent(Event{Name: "ping"}) {
		t.Error("TrackEvent on an uninitialised SDK returned true, want false")
	}
	if got := Dropped(); got != before {
		t.Errorf("Dropped() = %d, want %d — uninitialised must not count as a drop", got, before)
	}
}

// A full queue silently discarded the event and the bool return was ignored by
// every caller in practice. Dropped() and OnDrop are how that loss becomes
// visible.
func TestTrackEvent_FullQueueCountsAndReportsTheDrop(t *testing.T) {
	// A server that never answers keeps the single sender goroutine busy, so
	// the queue fills and stays full.
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer srv.Close()
	defer close(block)

	var mu sync.Mutex
	var dropsSeen []Event
	Init(Options{
		APIKey: "k", Endpoint: srv.URL, ProjectName: "p", QueueSize: 1,
		OnDrop: func(e Event) {
			mu.Lock()
			defer mu.Unlock()
			dropsSeen = append(dropsSeen, e)
		},
	})

	// Enqueue well past the capacity; at least one must be refused.
	var refused int
	for i := 0; i < 50; i++ {
		if !TrackEvent(Event{Name: "flood", SessionID: "s1"}) {
			refused++
		}
	}
	if refused == 0 {
		t.Fatal("nothing was refused with QueueSize=1 and a blocked sender")
	}
	if got := Dropped(); got != uint64(refused) {
		t.Errorf("Dropped() = %d, want %d", got, refused)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(dropsSeen) != refused {
		t.Fatalf("OnDrop fired %d times, want %d", len(dropsSeen), refused)
	}
	// The hook must receive the event that was lost, not a placeholder.
	if dropsSeen[0].Name != "flood" || dropsSeen[0].SessionID != "s1" {
		t.Errorf("OnDrop got %+v, want the original event", dropsSeen[0])
	}
}

func TestInit_ResetsTheDropCounter(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer srv.Close()
	defer close(block)

	Init(Options{APIKey: "k", Endpoint: srv.URL, ProjectName: "p", QueueSize: 1})
	for i := 0; i < 50; i++ {
		TrackEvent(Event{Name: "flood"})
	}
	if Dropped() == 0 {
		t.Fatal("expected drops before re-Init")
	}

	Init(Options{APIKey: "k", Endpoint: srv.URL, ProjectName: "p", QueueSize: 1})
	if got := Dropped(); got != 0 {
		t.Errorf("Dropped() after Init = %d, want 0", got)
	}
}
