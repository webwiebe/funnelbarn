package main

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
)

func TestWaitServer(t *testing.T) {
	srv := &http.Server{}

	closed := make(chan error, 1)
	closed <- http.ErrServerClosed
	if err := waitServer(context.Background(), srv, closed); err != nil {
		t.Fatalf("ErrServerClosed must end cleanly, got %v", err)
	}

	boom := errors.New("listen failed")
	failed := make(chan error, 1)
	failed <- boom
	if err := waitServer(context.Background(), srv, failed); !errors.Is(err, boom) {
		t.Fatalf("want listen error, got %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitServer(ctx, &http.Server{}, make(chan error)); err != nil {
		t.Fatalf("shutdown of an idle server must succeed, got %v", err)
	}
}

type countCommand struct{ n *int }

func (countCommand) Kind() string                  { return "count" }
func (c countCommand) Apply(context.Context) error { *c.n++; return nil }

func TestDrainCommandsAppliesQueued(t *testing.T) {
	d := command.New(command.Options{})
	d.Start(context.Background())
	var n int
	for i := 0; i < 3; i++ {
		d.Submit(context.Background(), countCommand{&n})
	}
	drainCommands(d)
	if n != 3 {
		t.Fatalf("drain applied %d of 3 commands", n)
	}
}
