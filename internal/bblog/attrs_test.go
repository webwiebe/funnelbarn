package bblog

import (
	"errors"
	"log/slog"
	"testing"
	"time"
)

func recordWith(attrs ...any) slog.Record {
	r := slog.NewRecord(time.Now(), slog.LevelError, "msg", 0)
	r.Add(attrs...)
	return r
}

// A group attribute reaches BugBarn as one dotted key per member, so the stall
// length of command.Outage shows on the issue.
func TestCaptureAttrs_FlattensGroups(t *testing.T) {
	r := recordWith("handled", true, slog.Group("outage", "total", "31s", "runs", 2))
	got, _ := captureAttrs(r)
	want := map[string]any{"handled": true, "outage.total": "31s", "outage.runs": int64(2)}
	if len(got) != len(want) {
		t.Fatalf("attrs = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("attrs[%q] = %v (%T), want %v", k, got[k], got[k], v)
		}
	}
}

func TestCaptureAttrs_NestedAndInlineGroups(t *testing.T) {
	r := recordWith(
		slog.Group("a", slog.Group("b", "c", "deep"), "d", "mid"),
		slog.Group("", "inline", "x"),
		slog.Group("empty"),
	)
	got, _ := captureAttrs(r)
	want := map[string]any{"a.b.c": "deep", "a.d": "mid", "inline": "x"}
	if len(got) != len(want) {
		t.Fatalf("attrs = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("attrs[%q] = %v, want %v", k, got[k], v)
		}
	}
}

// An error value is sent as text, and "err" is lifted for CaptureError, also
// when a group sits next to it.
func TestCaptureAttrs_ErrorsAsText(t *testing.T) {
	boom := errors.New("connection refused")
	r := recordWith("err", boom, slog.Group("g", "inner", errors.New("inner failed")))
	got, capErr := captureAttrs(r)
	if capErr != boom {
		t.Errorf("lifted error = %v, want %v", capErr, boom)
	}
	if got["err"] != "connection refused" || got["g.inner"] != "inner failed" {
		t.Errorf("attrs = %v, errors must be sent as text", got)
	}
}
