package funnelbarn

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// The regression this whole file exists for: before #237 every one of these
// statuses returned nil from send, so a permanently misconfigured SDK reported
// success on every event it never delivered.
func TestSend_NonSuccessStatusIsAnError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"wrong endpoint path", http.StatusNotFound, "404 page not found", "404"},
		{"missing or wrong key", http.StatusUnauthorized, `{"error":"invalid api key"}`, "401"},
		{"key for another project", http.StatusForbidden, "", "403"},
		{"payload too large", http.StatusRequestEntityTooLarge, "", "413"},
		{"server fault", http.StatusInternalServerError, "boom", "500"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			tr := newTransport(Options{APIKey: "k", Endpoint: srv.URL, QueueSize: 1})
			defer tr.shutdown(time.Second)

			err := tr.send(Event{Name: "signup"}.payload())
			if err == nil {
				t.Fatalf("send returned nil for %d — a rejected event must not look delivered", tc.status)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention status %s", err, tc.want)
			}
			// The status alone doesn't say which misconfiguration it is; the
			// server's own words do.
			if tc.body != "" && !strings.Contains(err.Error(), tc.body) {
				t.Errorf("error %q drops the response body %q", err, tc.body)
			}
		})
	}
}

func TestSend_2xxIsAccepted(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusAccepted, http.StatusNoContent} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		tr := newTransport(Options{APIKey: "k", Endpoint: srv.URL, QueueSize: 1})
		if err := tr.send(Event{Name: "signup"}.payload()); err != nil {
			t.Errorf("status %d: send = %v, want nil", status, err)
		}
		tr.shutdown(time.Second)
		srv.Close()
	}
}

// OnError is the hook a caller wires to find out. It must carry the event back
// intact — knowing "something failed" without knowing what is barely better
// than the old silence.
func TestOnError_ReceivesTheRejectedEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no such route", http.StatusNotFound)
	}))
	defer srv.Close()

	var mu sync.Mutex
	var got []Event
	var errs []error
	done := make(chan struct{}, 2)

	Init(Options{
		APIKey:   "k",
		Endpoint: srv.URL,
		OnError: func(e Event, err error) {
			mu.Lock()
			got = append(got, e)
			errs = append(errs, err)
			mu.Unlock()
			done <- struct{}{}
		},
	})

	when := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	TrackEvent(Event{
		Name:      "converted",
		UserID:    "lead-77607",
		SessionID: "send-abc123",
		Timestamp: when,
	})
	Track("signup", map[string]any{"plan": "pro"})
	if err := Shutdown(3 * time.Second); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for OnError")
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("OnError called %d times, want 2", len(got))
	}
	first := got[0]
	if first.Name != "converted" || first.SessionID != "send-abc123" || first.UserID != "lead-77607" {
		t.Errorf("OnError event = %+v, want the converted event with its join keys", first)
	}
	if !first.Timestamp.Equal(when) {
		t.Errorf("OnError timestamp = %v, want %v", first.Timestamp, when)
	}
	if !strings.Contains(errs[0].Error(), "404") {
		t.Errorf("OnError err = %v, want it to mention the 404", errs[0])
	}

	// Track's bool return only ever reported a full queue, so the counter is
	// the only place a rejection shows up for callers who don't set the hook.
	if n := Rejected(); n != 2 {
		t.Errorf("Rejected() = %d, want 2", n)
	}
	if n := Dropped(); n != 0 {
		t.Errorf("Dropped() = %d, want 0 — nothing was queue-dropped", n)
	}
}

func TestRejected_StaysZeroWhenEventsAreAccepted(t *testing.T) {
	bodies, srv := bodyCollector(t, 1)
	Init(Options{APIKey: "k", Endpoint: srv.URL})
	Track("signup", nil)
	if err := Shutdown(3 * time.Second); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	recv(t, bodies)
	if n := Rejected(); n != 0 {
		t.Errorf("Rejected() = %d, want 0", n)
	}
}

func TestInit_ResetsRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	failed := make(chan struct{}, 1)
	Init(Options{APIKey: "k", Endpoint: srv.URL, OnError: func(Event, error) { failed <- struct{}{} }})
	Track("signup", nil)
	<-failed
	if n := Rejected(); n == 0 {
		t.Fatal("Rejected() = 0 after a rejected event")
	}

	Init(Options{APIKey: "k", Endpoint: srv.URL})
	if n := Rejected(); n != 0 {
		t.Errorf("Rejected() = %d after Init, want 0", n)
	}
	_ = Shutdown(time.Second)
}

// Without a hook the SDK still has to say something once. A single stderr line
// is what would have caught two months of 404s.
func TestFirstFailureIsLoggedOnceWhenNoHookIsSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no such route", http.StatusNotFound)
	}))
	defer srv.Close()

	// Retire any transport a previous test left running against a closed
	// server, so its own (correct) first-failure line can't land in our buffer.
	_ = Shutdown(2 * time.Second)

	var buf bytes.Buffer
	oldOut, oldFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() { log.SetOutput(oldOut); log.SetFlags(oldFlags) }()

	Init(Options{APIKey: "k", Endpoint: srv.URL})
	for i := 0; i < 5; i++ {
		Track("signup", nil)
	}
	if err := Shutdown(3 * time.Second); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "404") {
		t.Errorf("log = %q, want the 404 reported", out)
	}
	if n := strings.Count(out, "ingest rejected"); n != 1 {
		t.Errorf("logged %d times, want exactly 1 — a broken endpoint must not spam the log", n)
	}
	if Rejected() != 5 {
		t.Errorf("Rejected() = %d, want 5 — every failure counts even though only one is logged", Rejected())
	}
}

func TestNoLogWhenOnErrorIsSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_ = Shutdown(2 * time.Second)

	var buf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(oldOut)

	seen := make(chan struct{}, 1)
	Init(Options{APIKey: "k", Endpoint: srv.URL, OnError: func(Event, error) { seen <- struct{}{} }})
	Track("signup", nil)
	<-seen
	if err := Shutdown(3 * time.Second); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if out := buf.String(); strings.Contains(out, "ingest rejected") {
		t.Errorf("log = %q, want silence — the caller took ownership by setting OnError", out)
	}
}

// The misconfiguration that started this: one config value for "the FunnelBarn
// endpoint", fed to a browser SDK and to this one. Both spellings must land on
// the same URL rather than one of them 404ing forever.
func TestNormaliseEndpoint(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"https://f.example.com", "https://f.example.com"},
		{"https://f.example.com/", "https://f.example.com"},
		{"  https://f.example.com  ", "https://f.example.com"},
		{"https://f.example.com/api/v1/events", "https://f.example.com"},
		{"https://f.example.com/api/v1/events/", "https://f.example.com"},
		{"https://f.example.com/api/v1/evaluate", "https://f.example.com"},
		{"https://f.example.com/funnelbarn/api/v1/events", "https://f.example.com/funnelbarn"},
		// Only the SDK's own suffix is stripped; an unrelated path is the
		// caller's mount point and must survive.
		{"https://f.example.com/events", "https://f.example.com/events"},
		{"", ""},
	} {
		if got := normaliseEndpoint(tc.in); got != tc.want {
			t.Errorf("normaliseEndpoint(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFullIngestURLAsEndpointStillDelivers(t *testing.T) {
	var paths []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.URL.Path != "/api/v1/events" {
			http.Error(w, "no such route", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	Init(Options{APIKey: "k", Endpoint: srv.URL + "/api/v1/events"})
	Track("signup", nil)
	if err := Shutdown(3 * time.Second); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 1 || paths[0] != "/api/v1/events" {
		t.Fatalf("posted to %v, want a single /api/v1/events", paths)
	}
	if n := Rejected(); n != 0 {
		t.Errorf("Rejected() = %d, want 0", n)
	}
}

func TestUnconfiguredEndpointIsReported(t *testing.T) {
	errs := make(chan error, 1)
	Init(Options{APIKey: "k", OnError: func(_ Event, err error) { errs <- err }})
	Track("signup", nil)
	select {
	case err := <-errs:
		if !strings.Contains(err.Error(), "endpoint") {
			t.Errorf("err = %v, want it to name the endpoint", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for OnError")
	}
	_ = Shutdown(time.Second)
}
