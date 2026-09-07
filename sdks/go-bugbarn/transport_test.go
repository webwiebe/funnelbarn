package bugbarn

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// notifyHandler is a minimal slog.Handler that signals a channel on every
// record so tests can wait deterministically instead of sleeping.
type notifyHandler struct {
	notify  chan slog.Record
	minimum slog.Level
}

func (h *notifyHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.minimum }
func (h *notifyHandler) Handle(_ context.Context, r slog.Record) error {
	h.notify <- r
	return nil
}
func (h *notifyHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *notifyHandler) WithGroup(string) slog.Handler      { return h }

func TestNormaliseEndpoint(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"bare root", "https://bugbarn.example.com", "https://bugbarn.example.com"},
		{"trailing slash", "https://bugbarn.example.com/", "https://bugbarn.example.com"},
		{"full ingest URL", "https://bugbarn.example.com/api/v1/events", "https://bugbarn.example.com"},
		{"full ingest URL with trailing slash", "https://bugbarn.example.com/api/v1/events/", "https://bugbarn.example.com"},
		{"whitespace", "  https://bugbarn.example.com  ", "https://bugbarn.example.com"},
		{"subpath root", "https://bugbarn.example.com/bugbarn", "https://bugbarn.example.com/bugbarn"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normaliseEndpoint(c.in); got != c.want {
				t.Errorf("normaliseEndpoint(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestTransportSend_AcceptsFullIngestURLAsEndpoint(t *testing.T) {
	paths := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Endpoint configured with the full ingest path already appended, as the
	// setup page used to hand out — must not double up to
	// /api/v1/events/api/v1/events.
	tr := newTransport("key", srv.URL+"/api/v1/events", "", 8)
	defer tr.shutdown(2 * time.Second)

	if err := tr.send(envelope{Timestamp: "now", SeverityText: "ERROR"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotPath := <-paths; gotPath != "/api/v1/events" {
		t.Fatalf("posted to %q, want /api/v1/events (not doubled up)", gotPath)
	}
}

func TestTransportSend_NonSuccessStatusIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("405 method not allowed"))
	}))
	defer srv.Close()

	tr := newTransport("key", srv.URL, "", 8)
	defer tr.shutdown(2 * time.Second)

	err := tr.send(envelope{Timestamp: "now", SeverityText: "ERROR"})
	if err == nil {
		t.Fatal("expected an error for a 405 response, got nil")
	}
	if !strings.Contains(err.Error(), "405") {
		t.Errorf("err = %v, want it to name the status", err)
	}
}

func TestTransport_LogsDeliveryFailureViaSlog(t *testing.T) {
	prev := slog.Default()
	h := &notifyHandler{notify: make(chan slog.Record, 8), minimum: slog.LevelWarn}
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(prev)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer srv.Close()

	tr := newTransport("key", srv.URL, "", 8)
	defer tr.shutdown(2 * time.Second)
	tr.enqueue(envelope{Timestamp: "now", SeverityText: "ERROR"})

	select {
	case rec := <-h.notify:
		if rec.Level != slog.LevelWarn {
			t.Fatalf("level = %v, want Warn", rec.Level)
		}
		if !strings.Contains(rec.Message, "delivery failed") {
			t.Fatalf("message = %q, want it to mention delivery failure", rec.Message)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the delivery-failure log")
	}
}

func TestTransport_SelfTest(t *testing.T) {
	bodies := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies <- body
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	prev := slog.Default()
	h := &notifyHandler{notify: make(chan slog.Record, 8), minimum: slog.LevelInfo}
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(prev)

	tr := newTransport("key", srv.URL, "", 8)
	defer tr.shutdown(2 * time.Second)

	tr.selfTest() // run synchronously for a deterministic assertion

	var received envelope
	if err := json.Unmarshal(<-bodies, &received); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if received.SeverityText != "INFO" {
		t.Fatalf("self-test event severity = %q, want INFO", received.SeverityText)
	}

	select {
	case rec := <-h.notify:
		if rec.Level != slog.LevelInfo {
			t.Fatalf("level = %v, want Info", rec.Level)
		}
		if !strings.Contains(rec.Message, "self-test succeeded") {
			t.Fatalf("message = %q, want it to report self-test success", rec.Message)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the self-test success log")
	}
}

func TestTransport_SelfTestFailureLogsError(t *testing.T) {
	prev := slog.Default()
	h := &notifyHandler{notify: make(chan slog.Record, 8), minimum: slog.LevelInfo}
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(prev)

	// Non-listening address: the self-test send fails outright.
	tr := newTransport("key", "http://127.0.0.1:1", "", 8)
	defer tr.shutdown(200 * time.Millisecond)

	tr.selfTest()

	select {
	case rec := <-h.notify:
		if rec.Level != slog.LevelError {
			t.Fatalf("level = %v, want Error", rec.Level)
		}
		if !strings.Contains(rec.Message, "self-test failed") {
			t.Fatalf("message = %q, want it to report self-test failure", rec.Message)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the self-test failure log")
	}
}

func TestTransportQueueFull(t *testing.T) {
	// Use a non-listening address so sends fail fast without blocking.
	// The transport background goroutine will error on each send and loop,
	// keeping items in the queue long enough to test back-pressure.
	const cap = 2
	tr := newTransport("key", "http://127.0.0.1:1", "", cap)
	defer tr.shutdown(200 * time.Millisecond)

	env := envelope{Timestamp: "now", SeverityText: "ERROR"}

	// Fill the buffered channel directly (bypass the goroutine draining it).
	tr.queue <- env
	tr.queue <- env

	// Queue is now at capacity; next enqueue must return false.
	if tr.enqueue(env) {
		t.Fatal("expected enqueue to return false when queue is full")
	}
}

func TestTransportSend(t *testing.T) {
	received := make(chan *http.Request, 1)
	var body []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		received <- r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr := newTransport("my-api-key", srv.URL, "my-project", 8)
	defer tr.shutdown(2 * time.Second)

	env := envelope{
		Timestamp:    "2024-01-01T00:00:00Z",
		SeverityText: "ERROR",
		Body:         "test error",
		Exception:    exceptionBlock{Type: "Error", Message: "test error"},
		Sender:       senderBlock{SDK: sdkBlock{Name: sdkName, Version: sdkVersion}},
	}
	tr.enqueue(env)

	select {
	case req := <-received:
		if req.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", req.Method)
		}
		if req.URL.Path != "/api/v1/events" {
			t.Fatalf("posted to %q, want /api/v1/events", req.URL.Path)
		}
		if ct := req.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("unexpected Content-Type: %s", ct)
		}
		if key := req.Header.Get("X-BugBarn-Api-Key"); key != "my-api-key" {
			t.Fatalf("unexpected api key: %s", key)
		}
		if proj := req.Header.Get("X-BugBarn-Project"); proj != "my-project" {
			t.Fatalf("unexpected project: %s", proj)
		}
		var parsed envelope
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatalf("invalid JSON body: %v", err)
		}
		if parsed.Body != "test error" {
			t.Fatalf("unexpected body: %s", parsed.Body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for request")
	}
}

func TestTransportShutdown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr := newTransport("key", srv.URL, "", 16)
	env := envelope{Timestamp: "now", SeverityText: "ERROR"}
	for i := 0; i < 5; i++ {
		tr.enqueue(env)
	}

	drained := tr.shutdown(2 * time.Second)
	if !drained {
		t.Fatal("expected shutdown to complete within timeout")
	}

	// done channel must be closed after shutdown.
	select {
	case <-tr.done:
		// ok
	default:
		t.Fatal("done channel not closed after shutdown")
	}
}
