package main

import (
	"testing"
	"time"
)

func TestTouchThrottle(t *testing.T) {
	th := newTouchThrottle(time.Minute, 2)
	t0 := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	if !th.due("a", t0) {
		t.Fatal("first touch of a key must be due")
	}
	if th.due("a", t0.Add(59*time.Second)) {
		t.Fatal("touch within the interval must be skipped")
	}
	if !th.due("a", t0.Add(time.Minute)) {
		t.Fatal("touch after the interval must be due")
	}
	if !th.due("b", t0) {
		t.Fatal("another key is throttled separately")
	}
	// The map holds a and b, its max; a third key resets it.
	if !th.due("c", t0) {
		t.Fatal("a new key past max must be due")
	}
	// a was touched at t0+1m; the reset forgot it, so it is due again early.
	if !th.due("a", t0.Add(70*time.Second)) {
		t.Fatal("a key forgotten by the reset must be due")
	}
}
