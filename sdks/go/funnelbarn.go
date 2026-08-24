// Package funnelbarn provides a Go SDK for sending analytics events to a
// FunnelBarn server.
package funnelbarn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Options configures the SDK.
type Options struct {
	APIKey      string
	Endpoint    string
	ProjectName string
	QueueSize   int // default 256

	// Environment tags every event ("production", "staging", "test",
	// "development"), so one key and project can be reused across deployments
	// and filtered apart in the dashboard. Aliases are normalised server-side;
	// an unrecognised value is filed under production and warned about.
	Environment string

	// FlagCacheTTL is how long a resolved flag is reused when the server sends
	// no cache hint of its own. Zero means DefaultFlagCacheTTL; negative
	// disables the cache entirely. A server that does advertise
	// cache_max_age_seconds always wins over this.
	FlagCacheTTL time.Duration

	// FlagKind is sent as "kind" on evaluation and is honoured only when the
	// call auto-registers a flag that does not exist yet: "config" for a
	// singleton value this service polls, "" or "experiment" otherwise.
	FlagKind string

	// OnDrop, when set, is called with the event that could not be queued
	// because the buffer was full.
	//
	// Dropping is the correct trade for page views and the wrong one for a
	// funnel-critical step: a discarded "converted" is indistinguishable from a
	// conversion that never happened. The bool return already reports it, but
	// in practice every caller ignores it, so the loss is invisible. This hook
	// (and Dropped) make it observable.
	//
	// It runs on the CALLER's goroutine, so keep it non-blocking — a hook that
	// does I/O turns a full buffer into backpressure on the very code path the
	// non-blocking enqueue exists to protect.
	OnDrop func(Event)
}

// eventPayload is the JSON body sent to POST /api/v1/events.
type eventPayload struct {
	Name        string         `json:"name"`
	URL         string         `json:"url,omitempty"`
	Referrer    string         `json:"referrer,omitempty"`
	UTMSource   string         `json:"utm_source,omitempty"`
	UTMMedium   string         `json:"utm_medium,omitempty"`
	UTMCampaign string         `json:"utm_campaign,omitempty"`
	UTMTerm     string         `json:"utm_term,omitempty"`
	UTMContent  string         `json:"utm_content,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
	UserID      string         `json:"user_id,omitempty"`
	SessionID   string         `json:"session_id,omitempty"`
	Timestamp   time.Time      `json:"timestamp"`
	Environment string         `json:"environment,omitempty"`
}

var (
	mu   sync.Mutex
	tp   *transport
	opts Options
)

// Init initialises the SDK. Safe to call multiple times; re-initialises.
func Init(o Options) {
	mu.Lock()
	defer mu.Unlock()
	if tp != nil {
		tp.shutdown(2 * time.Second)
	}
	if o.QueueSize <= 0 {
		o.QueueSize = 256
	}
	opts = o
	tp = newTransport(o)
	dropped.Store(0)
}

// Event is a fully-specified analytics event.
//
// It exists because Track and Page between them could only reach five of the
// fields the payload already carries. Everything else — session_id, user_id and
// the utm_* set — was marshalled and stored but unreachable from Go, which made
// one whole class of analytics impossible to build from a server: funnel
// reports key their steps on session_id, so events emitted without one can be
// recorded but never assembled into a funnel. See issue #223.
//
// A zero field is omitted from the payload, so the zero Event plus a Name
// behaves exactly like the old Track.
type Event struct {
	// Name is the event name, snake_case by convention. Required; an empty
	// Name is refused, matching Track.
	Name string

	URL      string
	Referrer string

	UTMSource   string
	UTMMedium   string
	UTMCampaign string
	UTMTerm     string
	UTMContent  string

	Properties map[string]any

	// UserID and SessionID are the join keys. SessionID is the one funnel
	// reports group on; a server-sent event that wants to take part in a funnel
	// must set it to something stable for the subject being followed (a
	// recipient, an order, a job) rather than leaving it empty.
	UserID    string
	SessionID string

	// Timestamp is when the event happened. Zero means now.
	//
	// Worth setting explicitly for anything replayed from a spool or a retry
	// queue: stamping at enqueue records when we got around to sending it, not
	// when it occurred.
	Timestamp time.Time
}

// payload converts to the wire shape, defaulting the timestamp.
func (e Event) payload() eventPayload {
	ts := e.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	return eventPayload{
		Name:        e.Name,
		URL:         e.URL,
		Referrer:    e.Referrer,
		UTMSource:   e.UTMSource,
		UTMMedium:   e.UTMMedium,
		UTMCampaign: e.UTMCampaign,
		UTMTerm:     e.UTMTerm,
		UTMContent:  e.UTMContent,
		Properties:  e.Properties,
		UserID:      e.UserID,
		SessionID:   e.SessionID,
		Timestamp:   ts.UTC(),
	}
}

// dropped counts events discarded because the queue was full. Reset by Init.
var dropped atomic.Uint64

// Dropped returns how many events have been discarded because the queue was
// full since the last Init. Enqueueing is non-blocking by design, so a busy
// producer loses events rather than stalling; this is how you find out.
func Dropped() uint64 { return dropped.Load() }

// TrackEvent sends a fully-specified event. Returns false if the SDK is not
// initialised, the Name is empty, or the queue was full.
func TrackEvent(e Event) bool {
	mu.Lock()
	t := tp
	onDrop := opts.OnDrop
	mu.Unlock()
	if t == nil || e.Name == "" {
		return false
	}
	if t.enqueue(e.payload()) {
		return true
	}
	// A full queue is a real loss, unlike "not initialised" above, which means
	// analytics was never switched on. Only the former is counted.
	dropped.Add(1)
	if onDrop != nil {
		onDrop(e)
	}
	return false
}

// Track sends a custom event.
func Track(name string, properties map[string]any) bool {
	return TrackEvent(Event{Name: name, Properties: properties})
}

// Page sends a page_view event.
func Page(url, referrer string) bool {
	return TrackEvent(Event{Name: "page_view", URL: url, Referrer: referrer})
}

// Flush waits for queued events to drain within the timeout.
func Flush() error {
	mu.Lock()
	t := tp
	mu.Unlock()
	if t == nil {
		return nil
	}
	if ok := t.flush(5 * time.Second); !ok {
		return fmt.Errorf("funnelbarn: flush timed out")
	}
	return nil
}

// Shutdown flushes and stops the background goroutine.
func Shutdown(timeout time.Duration) error {
	mu.Lock()
	t := tp
	tp = nil
	mu.Unlock()
	if t == nil {
		return nil
	}
	if ok := t.shutdown(timeout); !ok {
		return fmt.Errorf("funnelbarn: shutdown timed out")
	}
	return nil
}

// --------------------------------------------------------------------------
// Internal transport
// --------------------------------------------------------------------------

type transport struct {
	opts   Options
	queue  chan eventPayload
	done   chan struct{}
	client *http.Client
	flags  *flagCache
}

func newTransport(o Options) *transport {
	t := &transport{
		opts:   o,
		queue:  make(chan eventPayload, o.QueueSize),
		done:   make(chan struct{}),
		client: &http.Client{Timeout: 5 * time.Second},
		flags:  newFlagCache(),
	}
	go t.run()
	return t
}

func (t *transport) enqueue(e eventPayload) bool {
	// Stamped here so no event can escape untagged — an option that reaches
	// only some events makes the dashboard filter silently under-count.
	e.Environment = t.opts.Environment
	select {
	case t.queue <- e:
		return true
	default:
		return false
	}
}

func (t *transport) run() {
	defer close(t.done)
	for e := range t.queue {
		if err := t.send(e); err != nil {
			// Best-effort: drop on error.
			_ = err
		}
	}
}

func (t *transport) send(e eventPayload) error {
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}

	endpoint := t.opts.Endpoint
	if endpoint == "" {
		return fmt.Errorf("funnelbarn: endpoint not configured")
	}
	url := endpoint + "/api/v1/events"

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-funnelbarn-api-key", t.opts.APIKey)
	if t.opts.ProjectName != "" {
		req.Header.Set("x-funnelbarn-project", t.opts.ProjectName)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (t *transport) flush(timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			return false
		default:
			if len(t.queue) == 0 {
				return true
			}
			time.Sleep(10 * time.Millisecond)
		}
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
