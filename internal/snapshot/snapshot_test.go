package snapshot

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type warnCounter struct {
	mu    sync.Mutex
	warns int
	infos int
}

func (h *warnCounter) Enabled(context.Context, slog.Level) bool { return true }
func (h *warnCounter) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch r.Message {
	case "snapshot refresh failed":
		h.warns++
	case "snapshot refresh recovered":
		h.infos++
	}
	return nil
}
func (h *warnCounter) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *warnCounter) WithGroup(string) slog.Handler      { return h }

func captureLogs(t *testing.T) *warnCounter {
	t.Helper()
	h := &warnCounter{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return h
}

func TestSnapshot_ColdThenLoaded(t *testing.T) {
	c := &clock{now: time.Unix(1000, 0)}
	s := New(Config[int]{Name: "n", Interval: time.Second, Now: c.Now,
		Load: func(context.Context) (int, error) { return 7, nil }})
	if _, _, ok := s.Get(); ok {
		t.Fatal("a new snapshot must be cold")
	}
	c.advance(3 * time.Second)
	if got := s.Age(); got != 3*time.Second {
		t.Fatalf("cold age counts from New, got %v", got)
	}
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.advance(2 * time.Second)
	v, age, ok := s.Get()
	if !ok || v != 7 || age != 2*time.Second {
		t.Fatalf("Get = %v, %v, %v; want 7, 2s, true", v, age, ok)
	}
	if s.Name() != "n" {
		t.Errorf("Name = %q", s.Name())
	}
}

// A failed load keeps the last good value and lets its age grow, and the
// failure Warn is limited to one per failureLogInterval.
func TestSnapshot_FailedRefreshKeepsLastGood(t *testing.T) {
	logs := captureLogs(t)
	c := &clock{now: time.Unix(1000, 0)}
	fail := false
	s := New(Config[string]{Name: "n", Interval: time.Second, Now: c.Now,
		Load: func(context.Context) (string, error) {
			if fail {
				return "", errors.New("disk stall")
			}
			return "good", nil
		}})
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	fail = true
	for i := 0; i < 5; i++ {
		c.advance(5 * time.Second)
		if err := s.Refresh(context.Background()); err == nil {
			t.Fatal("want the load error back")
		}
	}
	v, age, ok := s.Get()
	if !ok || v != "good" || age != 25*time.Second {
		t.Fatalf("Get = %q, %v, %v; want the last good value at 25s", v, age, ok)
	}
	if logs.warns != 1 {
		t.Errorf("want 1 failure Warn within a minute, got %d", logs.warns)
	}
	c.advance(failureLogInterval)
	_ = s.Refresh(context.Background())
	if logs.warns != 2 {
		t.Errorf("want a second Warn after the interval, got %d", logs.warns)
	}
	fail = false
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if logs.infos != 1 {
		t.Errorf("want 1 recovery log, got %d", logs.infos)
	}
	if _, age, _ := s.Get(); age != 0 {
		t.Errorf("age after recovery = %v, want 0", age)
	}
}

// A refresh cancelled by shutdown is not a failure worth a Warn.
func TestSnapshot_CancelledRefreshDoesNotWarn(t *testing.T) {
	logs := captureLogs(t)
	s := New(Config[int]{Name: "n", Interval: time.Second,
		Load: func(ctx context.Context) (int, error) { return 0, ctx.Err() }})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Refresh(ctx); err == nil {
		t.Fatal("want the context error back")
	}
	if logs.warns != 0 {
		t.Errorf("want no Warn on shutdown, got %d", logs.warns)
	}
}

// The timeout ends a stalled load, so a later tick can retry.
func TestSnapshot_TimeoutEndsStalledLoad(t *testing.T) {
	captureLogs(t)
	s := New(Config[int]{Name: "n", Interval: time.Second, Timeout: 10 * time.Millisecond,
		Load: func(ctx context.Context) (int, error) {
			<-ctx.Done()
			return 0, ctx.Err()
		}})
	if err := s.Refresh(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
}

// Run loads right away and keeps loading on its own goroutine.
func TestSnapshot_RunRefreshesInBackground(t *testing.T) {
	loads := make(chan struct{}, 10)
	n := 0
	s := New(Config[int]{Name: "n", Interval: time.Millisecond,
		Load: func(context.Context) (int, error) {
			n++
			loads <- struct{}{}
			return n, nil
		}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Run(ctx)
	for i := 0; i < 2; i++ {
		select {
		case <-loads:
		case <-time.After(5 * time.Second):
			t.Fatalf("load %d did not happen", i+1)
		}
	}
	if v, _, ok := s.Get(); !ok || v < 1 {
		t.Fatalf("Get = %v, %v after two loads", v, ok)
	}
}
