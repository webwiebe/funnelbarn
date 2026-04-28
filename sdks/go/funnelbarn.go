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
	"time"
)

// Options configures the SDK.
type Options struct {
	APIKey      string
	Endpoint    string
	ProjectName string
	QueueSize   int // default 256
}

// eventPayload is the JSON body sent to POST /api/v1/events.
type eventPayload struct {
	Name        string         `json:"name"`
	URL         string         `json:"url,omitempty"`
	Referrer    string         `json:"referrer,omitempty"`
	UTMSource   string         `json:"utm_source,omitempty"`
	UTMMedium   string         `json:"utm_medium,omitempty"`
	UTMCampaign string         `json:"utm_campaign,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
	UserID      string         `json:"user_id,omitempty"`
	SessionID   string         `json:"session_id,omitempty"`
	Timestamp   time.Time      `json:"timestamp"`
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
}

// Track sends a custom event.
func Track(name string, properties map[string]any) bool {
	mu.Lock()
	t := tp
	mu.Unlock()
	if t == nil || name == "" {
		return false
	}
	return t.enqueue(eventPayload{
		Name:        name,
		Properties:  properties,
		Timestamp:   time.Now().UTC(),
	})
}

// Page sends a page_view event.
func Page(url, referrer string) bool {
	mu.Lock()
	t := tp
	mu.Unlock()
	if t == nil {
		return false
	}
	return t.enqueue(eventPayload{
		Name:      "page_view",
		URL:       url,
		Referrer:  referrer,
		Timestamp: time.Now().UTC(),
	})
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
}

func newTransport(o Options) *transport {
	t := &transport{
		opts:   o,
		queue:  make(chan eventPayload, o.QueueSize),
		done:   make(chan struct{}),
		client: &http.Client{Timeout: 5 * time.Second},
	}
	go t.run()
	return t
}

func (t *transport) enqueue(e eventPayload) bool {
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
