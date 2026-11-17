package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// InputCounts tallies the input fields of one type in a stored recording.
// No value is kept: a field is only classified.
type InputCounts struct {
	// Blocked fields carry the block class. rrweb records no value for them.
	Blocked int `json:"blocked"`
	// Masked fields hold a value made only of mask characters.
	Masked int `json:"masked"`
	// Empty fields hold no value.
	Empty int `json:"empty"`
	// Readable fields hold a value that is stored as typed.
	Readable int `json:"readable"`
	// EventsMasked and EventsReadable count the later input events (a user
	// typing) for fields of this type, by the same test.
	EventsMasked   int `json:"events_masked"`
	EventsReadable int `json:"events_readable"`
}

// InputCheck is what CheckInputs found in one recording.
type InputCheck struct {
	RecordingID string `json:"recording_id"`
	Page        string `json:"page,omitempty"`
	Chunks      int    `json:"chunks"`
	Unreadable  int    `json:"unreadable"`
	// Inputs are keyed by input type (text, email, password, textarea ...).
	Inputs map[string]*InputCounts `json:"inputs"`
	// Unmatched input events name a field the chunks read hold no node for.
	UnmatchedEvents int `json:"unmatched_events"`
	// BlockedElements counts every element carrying the block class.
	BlockedElements int `json:"blocked_elements"`
}

// ReadableTotal is the number of readable field values and typed events.
func (c InputCheck) ReadableTotal() int {
	n := 0
	for _, in := range c.Inputs {
		n += in.Readable + in.EventsReadable
	}
	return n
}

// Types returns the input types found, sorted.
func (c InputCheck) Types() []string {
	out := make([]string, 0, len(c.Inputs))
	for t := range c.Inputs {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// CheckInputs reads the stored chunks of one recording and reports, per input
// type, how many fields and typed values are blocked, masked, empty or
// readable. It prints and returns no field value.
func (svc *RecordingService) CheckInputs(ctx context.Context, recordingID string) (InputCheck, error) {
	rec, err := svc.store.GetRecording(ctx, recordingID)
	if errors.Is(err, sql.ErrNoRows) {
		return InputCheck{}, fmt.Errorf("%w: recording %s", domain.ErrNotFound, recordingID)
	}
	if err != nil {
		return InputCheck{}, fmt.Errorf("recordings: get recording: %w", err)
	}
	out := InputCheck{RecordingID: rec.ID, Page: rec.PageURL, Inputs: map[string]*InputCounts{}}
	w := &inputWalker{out: &out, types: map[int]string{}}
	upper := rec.LastChunkIndex
	if upper < rec.ChunkCount-1 {
		upper = rec.ChunkCount - 1
	}
	for i := rec.FirstChunkIndex; i <= upper; i++ {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		out.Chunks++
		events, ok := svc.readChunkEvents(ctx, chunkKey(rec.ProjectID, rec.ID, i))
		if !ok {
			out.Unreadable++
			continue
		}
		w.events(events)
	}
	return out, nil
}

// readChunkEvents loads and decodes one stored chunk.
func (svc *RecordingService) readChunkEvents(ctx context.Context, key string) ([]rrEvent, bool) {
	compressed, err := svc.storage.Get(ctx, key)
	if err != nil {
		return nil, false
	}
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, false
	}
	defer func() { _ = zr.Close() }()
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, false
	}
	var events []rrEvent
	if json.Unmarshal(raw, &events) != nil {
		return nil, false
	}
	return events, true
}

// rrEvent is the part of an rrweb event the check reads.
type rrEvent struct {
	Type int             `json:"type"`
	Data json.RawMessage `json:"data"`
}

// rrNode is a serialized DOM node of an rrweb snapshot or mutation.
type rrNode struct {
	ID          int                    `json:"id"`
	Type        int                    `json:"type"`
	TagName     string                 `json:"tagName"`
	Attributes  map[string]interface{} `json:"attributes"`
	TextContent string                 `json:"textContent"`
	ChildNodes  []rrNode               `json:"childNodes"`
}

// rrData holds the fields of the event kinds the check reads: a full
// snapshot (node), a mutation (adds) and an input event (source 5).
type rrData struct {
	Node   *rrNode `json:"node"`
	Source *int    `json:"source"`
	Adds   []struct {
		Node rrNode `json:"node"`
	} `json:"adds"`
	ID        int    `json:"id"`
	Text      string `json:"text"`
	IsChecked *bool  `json:"isChecked"`
}

const (
	rrFullSnapshot   = 2
	rrIncremental    = 3
	rrSourceMutation = 0
	rrSourceInput    = 5
	rrNodeElement    = 2
	rrNodeText       = 3
)

// inputWalker classifies the input fields of a recording across its chunks.
// types maps a node id to its input type, so a later input event is counted
// under the type of the field it names.
type inputWalker struct {
	out   *InputCheck
	types map[int]string
}

func (w *inputWalker) events(events []rrEvent) {
	for _, ev := range events {
		var d rrData
		if json.Unmarshal(ev.Data, &d) != nil {
			continue
		}
		switch {
		case ev.Type == rrFullSnapshot && d.Node != nil:
			w.node(*d.Node)
		case ev.Type == rrIncremental && d.Source != nil && *d.Source == rrSourceMutation:
			for _, add := range d.Adds {
				w.node(add.Node)
			}
		case ev.Type == rrIncremental && d.Source != nil && *d.Source == rrSourceInput:
			w.inputEvent(d)
		}
	}
}

func (w *inputWalker) counts(t string) *InputCounts {
	c := w.out.Inputs[t]
	if c == nil {
		c = &InputCounts{}
		w.out.Inputs[t] = c
	}
	return c
}

// node walks a subtree, classifying each input and textarea.
func (w *inputWalker) node(n rrNode) {
	if n.Type == rrNodeElement {
		_, blocked := n.Attributes["rr_width"]
		if blocked {
			w.out.BlockedElements++
		}
		switch strings.ToLower(n.TagName) {
		case "input":
			w.field(n, inputType(n), blocked, attrString(n, "value"))
		case "textarea":
			w.field(n, "textarea", blocked, childText(n))
		}
	}
	for _, c := range n.ChildNodes {
		w.node(c)
	}
}

func (w *inputWalker) field(n rrNode, t string, blocked bool, value string) {
	w.types[n.ID] = t
	c := w.counts(t)
	switch {
	case blocked:
		c.Blocked++
	case value == "":
		c.Empty++
	case isMasked(value):
		c.Masked++
	default:
		c.Readable++
	}
}

func (w *inputWalker) inputEvent(d rrData) {
	t, ok := w.types[d.ID]
	if !ok {
		w.out.UnmatchedEvents++
		return
	}
	if d.Text == "" || isMasked(d.Text) {
		w.counts(t).EventsMasked++
		return
	}
	w.counts(t).EventsReadable++
}

func inputType(n rrNode) string {
	t := strings.ToLower(attrString(n, "type"))
	if t == "" {
		return "text"
	}
	return t
}

func attrString(n rrNode, name string) string {
	v, _ := n.Attributes[name].(string)
	return v
}

// childText joins the text children of a node.
func childText(n rrNode) string {
	var b strings.Builder
	for _, c := range n.ChildNodes {
		if c.Type == rrNodeText {
			b.WriteString(c.TextContent)
		}
	}
	return b.String()
}

// isMasked reports a non-empty value made only of the mask character.
func isMasked(v string) bool {
	return v != "" && strings.Trim(v, "*") == ""
}
