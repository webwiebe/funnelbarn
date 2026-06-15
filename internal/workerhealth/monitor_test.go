package workerhealth

import (
	"testing"
	"time"
)

// fakeClock is a controllable time source for the monitor.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time      { return c.t }
func (c *fakeClock) add(d time.Duration) { c.t = c.t.Add(d) }

func newMonitor(clk *fakeClock, stallAfter time.Duration, geoSample int, geoReAlert time.Duration) *Monitor {
	return New(Options{StallAfter: stallAfter, GeoSampleSize: geoSample, GeoReAlert: geoReAlert, Now: clk.now})
}

func TestCheckProgress_FiresOnceWhenBacklogNotDraining(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	m := newMonitor(clk, 5*time.Minute, 200, time.Hour)

	// First observation just establishes a baseline.
	if alert, _ := m.CheckProgress(100, 50); alert {
		t.Fatal("first call must not alert")
	}
	// Offset frozen at 100 with pending data, but not yet past the window.
	clk.add(4 * time.Minute)
	if alert, _ := m.CheckProgress(100, 50); alert {
		t.Fatal("must not alert before StallAfter elapses")
	}
	// Past the window — should alert exactly once.
	clk.add(2 * time.Minute)
	alert, since := m.CheckProgress(100, 50)
	if !alert {
		t.Fatal("expected stall alert after StallAfter with pending data")
	}
	if since < 6*time.Minute {
		t.Errorf("stalledFor should reflect frozen duration, got %s", since)
	}
	// Still stalled, but must not re-alert.
	clk.add(10 * time.Minute)
	if alert, _ := m.CheckProgress(100, 50); alert {
		t.Fatal("must not re-alert while still stalled")
	}
}

func TestCheckProgress_NoAlertWhenIdle(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	m := newMonitor(clk, 1*time.Minute, 200, time.Hour)

	m.CheckProgress(100, 0)
	// Offset frozen but caught up (pending == 0) for a long time — not a stall.
	for i := 0; i < 5; i++ {
		clk.add(1 * time.Minute)
		if alert, _ := m.CheckProgress(100, 0); alert {
			t.Fatal("idle (no pending data) must never alert")
		}
	}
}

func TestCheckProgress_ReArmsAfterProgress(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	m := newMonitor(clk, 1*time.Minute, 200, time.Hour)

	m.CheckProgress(100, 50)
	clk.add(2 * time.Minute)
	if alert, _ := m.CheckProgress(100, 50); !alert {
		t.Fatal("expected first stall alert")
	}
	// Offset advances — re-arm.
	clk.add(1 * time.Minute)
	if alert, _ := m.CheckProgress(200, 50); alert {
		t.Fatal("progress must clear the stall, not alert")
	}
	// Freeze again — should be able to alert a second time.
	clk.add(2 * time.Minute)
	if alert, _ := m.CheckProgress(200, 50); !alert {
		t.Fatal("expected a fresh stall alert after re-arming")
	}
}

func TestRecordGeo_AlertsOnAllMisses(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	m := newMonitor(clk, time.Minute, 10, 6*time.Hour)

	var fired int
	for i := 0; i < 10; i++ {
		if alert, n := m.RecordGeo(false); alert {
			fired++
			if n != 10 {
				t.Errorf("attempts: want 10, got %d", n)
			}
		}
	}
	if fired != 1 {
		t.Fatalf("expected exactly one geo alert for a zero-hit window, got %d", fired)
	}
}

func TestRecordGeo_NoAlertWhenSomeHits(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	m := newMonitor(clk, time.Minute, 10, 6*time.Hour)

	for i := 0; i < 9; i++ {
		if alert, _ := m.RecordGeo(false); alert {
			t.Fatal("should not alert before the window completes")
		}
	}
	// One hit in the window — no alert.
	if alert, _ := m.RecordGeo(true); alert {
		t.Fatal("a window with at least one hit must not alert")
	}
}

func TestRecordGeo_ThrottlesReAlerts(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	m := newMonitor(clk, time.Minute, 2, 6*time.Hour)

	// Window 1: two misses -> alert.
	m.RecordGeo(false)
	if alert, _ := m.RecordGeo(false); !alert {
		t.Fatal("expected alert on first zero-hit window")
	}
	// Window 2 immediately after: still all misses, but within re-alert window.
	m.RecordGeo(false)
	if alert, _ := m.RecordGeo(false); alert {
		t.Fatal("must throttle re-alerts within GeoReAlert")
	}
	// After the re-alert window passes, it may alert again.
	clk.add(7 * time.Hour)
	m.RecordGeo(false)
	if alert, _ := m.RecordGeo(false); !alert {
		t.Fatal("expected a re-alert after GeoReAlert elapsed")
	}
}
