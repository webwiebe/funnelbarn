package bblog_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	bb "github.com/wiebe-xyz/bugbarn-go"

	"github.com/wiebe-xyz/funnelbarn/internal/bblog"
)

// selfTestBody is the one envelope bb.Init posts on its own, to prove the
// endpoint and key work. It is not a captured record and must not be counted
// as one.
const selfTestBody = "bugbarn self-reporting initialised"

// fakeBugBarn is a stand-in for BugBarn's ingest endpoint that keeps every
// envelope posted to it.
type fakeBugBarn struct {
	*httptest.Server
	mu   sync.Mutex
	seen []map[string]any
}

func newFakeBugBarn(t *testing.T) *fakeBugBarn {
	t.Helper()
	f := &fakeBugBarn{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env map[string]any
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			t.Errorf("decode envelope: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.seen = append(f.seen, env)
		f.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(f.Close)
	return f
}

// captured returns the envelopes that came from a log record, excluding the
// startup self-test.
func (f *fakeBugBarn) captured() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]map[string]any, 0, len(f.seen))
	for _, env := range f.seen {
		if env["body"] != selfTestBody {
			out = append(out, env)
		}
	}
	return out
}

// awaitSelfTest blocks until bb.Init's self-test envelope has arrived, so that
// a late-delivered one cannot be mistaken for a captured record.
func (f *fakeBugBarn) awaitSelfTest(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		for _, env := range f.seen {
			if env["body"] == selfTestBody {
				f.mu.Unlock()
				return
			}
		}
		f.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("self-test envelope never arrived; the fake endpoint is not wired up")
}

// initSDK points the SDK at f and returns a logger wrapped by bblog. Shutdown
// (not Flush) is what the caller must use to drain: Flush returns as soon as
// the queue is empty, which is before the last send has finished.
func initSDK(t *testing.T, f *fakeBugBarn) *slog.Logger {
	t.Helper()
	bb.Init(bb.Options{APIKey: "test-key", Endpoint: f.URL, ProjectSlug: "bblog-test"})
	f.awaitSelfTest(t)
	return slog.New(bblog.NewHandler(&recordingHandler{level: slog.LevelDebug}))
}

// Warn records must not reach BugBarn. BugBarn opens an issue for every event
// it ingests, with no severity gate, so a forwarded warning is indistinguishable
// from a crash: it opens an issue that has to be resolved by hand. On
// 2026-09-08 six of the eleven open FunnelBarn issues were warnings about
// things callers did — a crawler with no API key, a key whose project had been
// deleted, a doubled request path — and three were still arriving from
// AhrefsBot. This is the assertion the old branch-coverage tests could not
// make: they never initialised the SDK, so they passed either way.
func TestHandler_DoesNotForwardWarnOrBelow(t *testing.T) {
	f := newFakeBugBarn(t)
	logger := initSDK(t, f)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("recording chunk: unauthorized", "user_agent", "AhrefsBot/7.0")
	logger.Warn("service error: not found", "err", errors.New("not found"))

	if !bb.Shutdown(5 * time.Second) {
		t.Fatal("SDK did not drain within 5s")
	}
	if got := f.captured(); len(got) != 0 {
		t.Errorf("want nothing below Error forwarded to BugBarn, got %d envelope(s): %v", len(got), got)
	}
}

// Error records must still reach BugBarn — narrowing the handler to Error is
// only correct if Error itself still gets through.
func TestHandler_ForwardsErrorWithMessageAndAttributes(t *testing.T) {
	f := newFakeBugBarn(t)
	logger := initSDK(t, f)

	logger.Error("worker dead-lettering record", "ingest_id", "cd4b0861", "handled", false)

	if !bb.Shutdown(5 * time.Second) {
		t.Fatal("SDK did not drain within 5s")
	}
	got := f.captured()
	if len(got) != 1 {
		t.Fatalf("want 1 envelope forwarded, got %d: %v", len(got), got)
	}
	if body, want := got[0]["body"], "[ERROR] worker dead-lettering record"; body != want {
		t.Errorf("body = %v, want %q", body, want)
	}
	attrs, ok := got[0]["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("attributes missing or not an object: %v", got[0]["attributes"])
	}
	if attrs["ingest_id"] != "cd4b0861" {
		t.Errorf("ingest_id attribute = %v, want %q", attrs["ingest_id"], "cd4b0861")
	}
	if attrs["handled"] != false {
		t.Errorf("handled attribute = %v, want false", attrs["handled"])
	}
}

// An Error carrying an "err" attribute takes the CaptureError path, which sends
// the error's own type and a stack trace so BugBarn can group by fingerprint.
func TestHandler_ForwardsErrorAttributeAsException(t *testing.T) {
	f := newFakeBugBarn(t)
	logger := initSDK(t, f)

	logger.Error("count orphaned rows", "err", errors.New("orphaned rows present"))

	if !bb.Shutdown(5 * time.Second) {
		t.Fatal("SDK did not drain within 5s")
	}
	got := f.captured()
	if len(got) != 1 {
		t.Fatalf("want 1 envelope forwarded, got %d: %v", len(got), got)
	}
	// CaptureError sends the error value, not the log message, so BugBarn
	// fingerprints on the error and its stack.
	if body, want := got[0]["body"], "orphaned rows present"; body != want {
		t.Errorf("body = %v, want %q", body, want)
	}
	exc, ok := got[0]["exception"].(map[string]any)
	if !ok {
		t.Fatalf("exception missing or not an object: %v", got[0]["exception"])
	}
	if frames, ok := exc["stacktrace"].([]any); !ok || len(frames) == 0 {
		t.Errorf("want a stacktrace on the CaptureError path, got %v", exc["stacktrace"])
	}
}

// Every record still reaches the wrapped handler, whatever its level — dropping
// warnings from BugBarn must not drop them from the logs.
func TestHandler_PassesAllLevelsToBaseHandler(t *testing.T) {
	f := newFakeBugBarn(t)
	bb.Init(bb.Options{APIKey: "test-key", Endpoint: f.URL, ProjectSlug: "bblog-test"})
	f.awaitSelfTest(t)

	base := &recordingHandler{level: slog.LevelDebug}
	logger := slog.New(bblog.NewHandler(base))

	logger.Debug("d")
	logger.Info("i")
	logger.Warn("w")
	logger.Error("e")

	if !bb.Shutdown(5 * time.Second) {
		t.Fatal("SDK did not drain within 5s")
	}
	if len(base.records) != 4 {
		t.Errorf("want all 4 records passed to the base handler, got %d", len(base.records))
	}
}
