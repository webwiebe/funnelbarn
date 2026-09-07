package bugbarn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// eventsPath is the ingest endpoint every envelope is posted to. Endpoint is
// meant to be the server root ("https://bugbarn.example.com") with this path
// appended by the transport — but nothing enforced that, so a caller handing
// in the full ingest URL (copied from a project's setup page, or from a
// sibling SDK's config) silently posted every event to the bare root and got
// a 405 back that was never logged. See issue #259.
const eventsPath = "/api/v1/events"

// logRateLimit is the minimum gap between consecutive "delivery failing"
// warnings. A misconfigured endpoint fails every single send, and logging
// each one would spam stderr as hard as the silence it replaces; one line per
// window is enough to notice the problem and stay findable in the logs.
const logRateLimit = time.Minute

type transport struct {
	apiKey      string
	endpoint    string
	projectSlug string
	queue       chan envelope
	done        chan struct{}
	client      *http.Client

	logMu      sync.Mutex
	lastLogged time.Time
}

func newTransport(apiKey, endpoint, projectSlug string, queueSize int) *transport {
	t := &transport{
		apiKey:      apiKey,
		endpoint:    normaliseEndpoint(endpoint),
		projectSlug: projectSlug,
		queue:       make(chan envelope, queueSize),
		done:        make(chan struct{}),
		client:      &http.Client{Timeout: 5 * time.Second},
	}
	go t.run()
	return t
}

// normaliseEndpoint reduces whatever was configured to a base URL. Endpoint
// is meant to be the server root — the transport appends eventsPath itself —
// but a full ingest URL is accepted too, so that the same "BugBarn endpoint"
// value handed to a browser SDK and to this one both work.
func normaliseEndpoint(endpoint string) string {
	e := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	return strings.TrimSuffix(e, eventsPath)
}

func (t *transport) enqueue(env envelope) bool {
	select {
	case t.queue <- env:
		return true
	default:
		return false
	}
}

func (t *transport) run() {
	defer close(t.done)
	for env := range t.queue {
		if err := t.send(env); err != nil {
			t.logSendErr(err)
		}
	}
}

// logSendErr reports a failed delivery via slog, rate-limited so a
// permanently broken endpoint logs steadily instead of either flooding the
// log or (as before) never logging at all.
func (t *transport) logSendErr(err error) {
	t.logMu.Lock()
	defer t.logMu.Unlock()
	if time.Since(t.lastLogged) < logRateLimit {
		return
	}
	t.lastLogged = time.Now()
	slog.Warn("bugbarn: event delivery failed", "err", err)
}

// selfTest sends one info-level event synchronously (bypassing the queue) so
// a misconfigured endpoint or API key is reported once, loudly, at startup —
// instead of surfacing only as a rate-limited trickle of warnings once real
// error traffic starts arriving, or (as before this fix) not surfacing at
// all. Meant to be run in its own goroutine so it never blocks Init.
func (t *transport) selfTest() {
	if err := t.send(buildSelfTestEnvelope()); err != nil {
		slog.Error("bugbarn: self-test failed; self-reporting is misconfigured and events will not reach BugBarn", "err", err)
		return
	}
	slog.Info("bugbarn: self-test succeeded")
}

func (t *transport) flush(timeout time.Duration) bool {
	// Drain by sending a sentinel that closes after queue is empty.
	// Simple approach: close the queue and wait for done with timeout.
	// We can't close here (flush may be called multiple times).
	// Instead: create a ticker-based drain check.
	deadline := time.Now().Add(timeout)
	for {
		if len(t.queue) == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (t *transport) shutdown(timeout time.Duration) bool {
	close(t.queue)
	select {
	case <-t.done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (t *transport) send(env envelope) error {
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if t.endpoint == "" {
		return fmt.Errorf("bugbarn: endpoint not configured")
	}
	url := t.endpoint + eventsPath

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-BugBarn-Api-Key", t.apiKey)
	if t.projectSlug != "" {
		req.Header.Set("X-BugBarn-Project", t.projectSlug)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// A 404 (wrong endpoint) or 405 (bare root, no path) used to return nil
	// here — the whole reason this bug went unnoticed for two months. Include
	// a slice of the body so the log line says which of those it is.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if detail := strings.TrimSpace(string(snippet)); detail != "" {
			return fmt.Errorf("bugbarn: ingest rejected the event: %s: %s", resp.Status, detail)
		}
		return fmt.Errorf("bugbarn: ingest rejected the event: %s", resp.Status)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return nil
}
