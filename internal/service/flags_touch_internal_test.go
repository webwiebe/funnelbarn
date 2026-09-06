package service

import (
	"testing"
	"time"
)

// The throttle replaces an origin filter as the defence against write
// amplification, so it has to hold on its own: one write per flag per minute,
// independently per flag, and never a permanent lockout.
func TestShouldTouch_ThrottlesPerFlagPerInterval(t *testing.T) {
	svc := NewFlagService(nil)
	base := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	if !svc.shouldTouch("proj-1", "flag-a", base) {
		t.Fatal("first evaluation should write")
	}
	if svc.shouldTouch("proj-1", "flag-a", base.Add(time.Second)) {
		t.Error("second evaluation inside the interval should be skipped")
	}
	if svc.shouldTouch("proj-1", "flag-a", base.Add(touchInterval-time.Nanosecond)) {
		t.Error("an evaluation just under the interval should be skipped")
	}
	if !svc.shouldTouch("proj-1", "flag-a", base.Add(touchInterval)) {
		t.Error("an evaluation at the interval should write again")
	}

	// A different flag has its own budget.
	if !svc.shouldTouch("proj-1", "flag-b", base.Add(time.Second)) {
		t.Error("a different flag key should not be throttled by flag-a")
	}
	// So does the same key in a different project — the throttle must not let
	// one project suppress another's writes.
	if !svc.shouldTouch("proj-2", "flag-a", base.Add(time.Second)) {
		t.Error("a different project should not be throttled by proj-1")
	}
}

// The throttle map is bounded, and dropping it costs at most one extra write.
func TestShouldTouch_BoundsTheThrottleMap(t *testing.T) {
	svc := NewFlagService(nil)
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	for i := range maxTrackedTouches + 10 {
		if !svc.shouldTouch("proj-1", "flag-"+string(rune('a'+i%26))+string(rune(i)), now) {
			t.Fatalf("distinct flag %d should write", i)
		}
	}
	svc.touchedMu.Lock()
	size := len(svc.touchedAt)
	svc.touchedMu.Unlock()
	if size > maxTrackedTouches {
		t.Errorf("throttle map grew to %d, past the %d cap", size, maxTrackedTouches)
	}
}
