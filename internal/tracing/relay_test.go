package tracing

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestSpansURL(t *testing.T) {
	cases := map[string]string{
		"https://spanbarn.wiebe.xyz/v1/traces":  "https://spanbarn.wiebe.xyz/api/v1/spans",
		"https://spanbarn.wiebe.xyz/v1/traces/": "https://spanbarn.wiebe.xyz/api/v1/spans",
		"https://spanbarn.wiebe.xyz":            "https://spanbarn.wiebe.xyz/api/v1/spans",
		"http://localhost:4318/v1/traces":       "http://localhost:4318/api/v1/spans",
	}
	for in, want := range cases {
		if got := spansURL(in); got != want {
			t.Errorf("spansURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// Telemetry export off means the relay is nil, and a nil relay must be safe to
// call — the dashboard still runs when SpanBarn is not configured.
func TestNewSpanRelay_NilWhenUnconfigured(t *testing.T) {
	for _, cfg := range []Config{{}, {Endpoint: "x"}, {APIKey: "y"}} {
		r := NewSpanRelay(cfg)
		if r != nil {
			t.Fatalf("expected nil relay for %+v", cfg)
		}
		r.Enqueue([]BrowserSpan{{TraceID: "t"}}) // must not panic
		if r.Enabled() {
			t.Error("a nil relay is not enabled")
		}
		if r.Dropped() != 0 {
			t.Error("a nil relay drops nothing")
		}
		r.Shutdown()
	}
}

// The browser's span IDs must survive the relay untouched: a CLIENT span whose
// ID was rewritten would no longer parent the server span that quoted it in
// the traceparent header, and the trace would come apart.
func TestSpanRelay_ForwardsSpansVerbatim(t *testing.T) {
	type batch struct {
		Spans []BrowserSpan `json:"spans"`
	}
	got := make(chan batch, 1)
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		path = r.URL.Path
		var b batch
		_ = json.NewDecoder(r.Body).Decode(&b)
		got <- b
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	relay := NewSpanRelay(Config{Endpoint: srv.URL + "/v1/traces", APIKey: "secret"})
	if relay == nil {
		t.Fatal("expected a relay")
	}
	span := BrowserSpan{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", SpanID: "00f067aa0ba902b7",
		ParentSpanID: "0102030405060708", Name: "GET /api/v1/funnels",
		Service: "funnelbarn-web", Kind: "CLIENT", Status: "OK",
		StartTime: 1_700_000_000_000_000, Duration: 1234,
		Attributes: map[string]any{"http.status_code": float64(200)},
	}
	relay.Enqueue([]BrowserSpan{span})
	relay.Shutdown()

	select {
	case b := <-got:
		if len(b.Spans) != 1 {
			t.Fatalf("want 1 span, got %d", len(b.Spans))
		}
		if !reflect.DeepEqual(b.Spans[0], span) {
			t.Errorf("span mutated in transit:\n got %+v\nwant %+v", b.Spans[0], span)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the relayed batch")
	}

	if auth != "Bearer secret" {
		t.Errorf("Authorization: got %q", auth)
	}
	if path != "/api/v1/spans" {
		t.Errorf("path: want SpanBarn's native ingest, got %q", path)
	}
}

// A SpanBarn outage must cost dropped telemetry, not a failed dashboard request.
func TestSpanRelay_SurvivesIngestFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	relay := NewSpanRelay(Config{Endpoint: srv.URL + "/v1/traces", APIKey: "k"})
	relay.Enqueue([]BrowserSpan{{TraceID: "t", SpanID: "s"}})
	relay.Shutdown() // must return rather than hang or panic
}

func TestSpanRelay_EmptyBatchIsNotSent(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	relay := NewSpanRelay(Config{Endpoint: srv.URL + "/v1/traces", APIKey: "k"})
	relay.Enqueue(nil)
	relay.Enqueue([]BrowserSpan{})
	relay.Shutdown()

	if calls != 0 {
		t.Errorf("an empty batch must not be sent, got %d calls", calls)
	}
}
