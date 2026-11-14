package pipeline

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
)

// flakyPublisher fails the next publish when told to.
type flakyPublisher struct{ failNext bool }

func (p *flakyPublisher) PublishBatch(context.Context, [][]byte) error {
	if p.failNext {
		p.failNext = false
		return errors.New("connection refused")
	}
	return nil
}

// A reader's buffered commands can keep failing to forward with no other Error
// on the way, so the forwarder reports repeated stalls itself, once per window.
func TestFallbackForwarderRepeatedStallsLogErrorOnce(t *testing.T) {
	const (
		failed    = "bookkeeping: forwarding buffered commands failed"
		recovered = "bookkeeping: forwarding buffered commands recovered"
	)
	count := levelCounts(t)
	ctx := context.Background()
	fallback, err := command.NewSpoolFallback(t.TempDir(), "bookkeeping", 0, slog.Default())
	require.NoError(t, err)
	now := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	list := &flakyPublisher{}
	f := NewFallbackForwarder("bookkeeping", fallback, list)
	f.outage.Now = func() time.Time { return now }

	stall := func() {
		fallback.Submit(ctx, command.TouchAPIKey{KeyHash: "k"})
		list.failNext = true
		f.Forward(ctx)
		now = now.Add(29 * time.Second)
		list.failNext = true
		f.Forward(ctx)
		now = now.Add(time.Second)
		f.Forward(ctx)
	}

	stall()
	require.Equal(t, 1, count("WARN", failed))
	require.Zero(t, count("ERROR", failed), "29s is under the total")
	require.Equal(t, 1, count("INFO", recovered))

	stall()
	require.Equal(t, 1, count("ERROR", failed), "the second run takes the total to 30s")

	stall()
	stall()
	require.Equal(t, 1, count("ERROR", failed), "once inside the window")
	require.Equal(t, 4, count("INFO", recovered))

	now = now.Add(command.OutageWindow)
	stall()
	stall()
	require.Equal(t, 2, count("ERROR", failed), "the pattern goes on past the window")
}
