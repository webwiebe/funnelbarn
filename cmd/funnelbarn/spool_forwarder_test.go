package main

import (
	"context"
	"errors"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

// fakeForwardList records what is published, in order.
type fakeForwardList struct {
	queued    int64
	published []string // ingest ids
	failNext  bool
}

func (l *fakeForwardList) PublishBatch(_ context.Context, payloads [][]byte) error {
	if l.failNext {
		l.failNext = false
		return errors.New("connection refused")
	}
	for _, p := range payloads {
		c, _, err := command.Decode(p)
		if err != nil {
			return err
		}
		l.published = append(l.published, c.(command.IngestRecord).Record.IngestID)
	}
	l.queued += int64(len(payloads))
	return nil
}

func (l *fakeForwardList) QueuedLen(context.Context) (int64, error) { return l.queued, nil }

func entries(ids ...string) []spool.RecordAtOffset {
	out := make([]spool.RecordAtOffset, len(ids))
	for i, id := range ids {
		out[i] = spool.RecordAtOffset{Record: spool.Record{IngestID: id}, EndOffset: int64(100 * (i + 1))}
		if id == "" {
			out[i].Malformed = errors.New("bad json")
		}
	}
	return out
}

func cursor(t *testing.T, dir string) int64 {
	t.Helper()
	n, err := spool.ReadCursor(dir)
	if err != nil {
		t.Fatalf("ReadCursor: %v", err)
	}
	return n
}

func TestForwardPublishesInOrderAndMovesCursor(t *testing.T) {
	dir := t.TempDir()
	list := &fakeForwardList{}
	f := &spoolForwarder{list: list, spoolDir: dir, maxLen: 100}

	got := f.forward(context.Background(), entries("a", "b", "c"), 0)
	if got != 300 || cursor(t, dir) != 300 {
		t.Fatalf("offset = %d, cursor = %d, want 300", got, cursor(t, dir))
	}
	if want := []string{"a", "b", "c"}; !equal(list.published, want) {
		t.Errorf("published %v, want %v", list.published, want)
	}
}

// A failed publish leaves the cursor where it was, so the records are sent
// again on the next tick.
func TestForwardKeepsRecordsInSpoolWhenPublishFails(t *testing.T) {
	dir := t.TempDir()
	list := &fakeForwardList{failNext: true}
	f := &spoolForwarder{list: list, spoolDir: dir, maxLen: 100}

	if got := f.forward(context.Background(), entries("a", "b"), 0); got != 0 {
		t.Fatalf("offset = %d after a failed publish, want 0", got)
	}
	if cursor(t, dir) != 0 {
		t.Fatalf("cursor moved after a failed publish")
	}
	if got := f.forward(context.Background(), entries("a", "b"), 0); got != 200 {
		t.Fatalf("offset = %d on retry, want 200", got)
	}
	if want := []string{"a", "b"}; !equal(list.published, want) {
		t.Errorf("published %v, want %v", list.published, want)
	}
}

// At the cap the forwarder sends only what fits and leaves the rest in the
// spool.
func TestForwardStopsAtTheCap(t *testing.T) {
	dir := t.TempDir()
	list := &fakeForwardList{queued: 8}
	f := &spoolForwarder{list: list, spoolDir: dir, maxLen: 10}

	if got := f.forward(context.Background(), entries("a", "b", "c"), 0); got != 200 {
		t.Fatalf("offset = %d, want 200 (two records fit)", got)
	}
	if got := f.forward(context.Background(), entries("c"), 200); got != 200 {
		t.Fatalf("offset = %d with a full queue, want 200", got)
	}
	if want := []string{"a", "b"}; !equal(list.published, want) {
		t.Errorf("published %v, want %v", list.published, want)
	}
}

// A malformed line is skipped in place; the records around it keep their order.
func TestForwardSkipsMalformedLine(t *testing.T) {
	dir := t.TempDir()
	list := &fakeForwardList{}
	f := &spoolForwarder{list: list, spoolDir: dir, maxLen: 100}

	if got := f.forward(context.Background(), entries("a", "", "c"), 0); got != 300 {
		t.Fatalf("offset = %d, want 300", got)
	}
	if want := []string{"a", "c"}; !equal(list.published, want) {
		t.Errorf("published %v, want %v", list.published, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
