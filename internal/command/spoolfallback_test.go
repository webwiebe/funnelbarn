package command_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
)

// recordingPublisher collects what Forward publishes, and fails while fail is set.
type recordingPublisher struct {
	got  []command.Command
	fail bool
}

func (p *recordingPublisher) publish(_ context.Context, payloads [][]byte) error {
	if p.fail {
		return errors.New("valkey down")
	}
	for _, b := range payloads {
		c, _, err := command.Decode(b)
		if err != nil {
			return err
		}
		p.got = append(p.got, c)
	}
	return nil
}

func TestSpoolFallbackForwardsAfterAnOutage(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	f, err := command.NewSpoolFallback(dir, "bookkeeping-fallback", 0, nil)
	require.NoError(t, err)
	defer f.Close(ctx)

	f.Submit(ctx, command.TouchAPIKey{KeyHash: "k1"})
	f.Submit(ctx, command.MarkProjectHealth{ProjectID: "p1", Field: command.HealthEventsReceived})

	pub := &recordingPublisher{fail: true}
	n, err := f.Forward(ctx, pub.publish)
	require.Error(t, err)
	require.Zero(t, n)

	pub.fail = false
	n, err = f.Forward(ctx, pub.publish)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, []command.Command{
		command.TouchAPIKey{KeyHash: "k1"},
		command.MarkProjectHealth{ProjectID: "p1", Field: command.HealthEventsReceived},
	}, pub.got)

	// Drained: the file is emptied and nothing is sent twice.
	info, err := os.Stat(filepath.Join(dir, "bookkeeping-fallback.ndjson"))
	require.NoError(t, err)
	require.Zero(t, info.Size())
	n, err = f.Forward(ctx, pub.publish)
	require.NoError(t, err)
	require.Zero(t, n)

	f.Submit(ctx, command.TouchAPIKey{KeyHash: "k2"})
	n, err = f.Forward(ctx, pub.publish)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, command.TouchAPIKey{KeyHash: "k2"}, pub.got[2])
}

func TestSpoolFallbackSurvivesARestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	f, err := command.NewSpoolFallback(dir, "fb", 0, nil)
	require.NoError(t, err)
	f.Submit(ctx, command.TouchAPIKey{KeyHash: "k1"})
	require.NoError(t, f.Close(ctx))

	// A crash cut the next write short.
	file, err := os.OpenFile(filepath.Join(dir, "fb.ndjson"), os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = file.WriteString(`{"kind":"touch_api_key","pay`)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	f, err = command.NewSpoolFallback(dir, "fb", 0, nil)
	require.NoError(t, err)
	defer f.Close(ctx)
	f.Submit(ctx, command.TouchAPIKey{KeyHash: "k2"})

	var payloads [][]byte
	n, err := f.Forward(ctx, func(_ context.Context, p [][]byte) error {
		payloads = append(payloads, p...)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, n)
	_, _, err = command.Decode(payloads[1])
	require.Error(t, err, "the torn line is sent as it is and dropped by the consumer")
	c, _, err := command.Decode(payloads[2])
	require.NoError(t, err)
	require.Equal(t, command.TouchAPIKey{KeyHash: "k2"}, c)
}

func TestSpoolFallbackStopsAtItsCap(t *testing.T) {
	ctx := context.Background()
	f, err := command.NewSpoolFallback(t.TempDir(), "fb", 100, nil)
	require.NoError(t, err)
	defer f.Close(ctx)
	for range 10 {
		f.Submit(ctx, command.TouchAPIKey{KeyHash: "k"})
	}
	pub := &recordingPublisher{}
	n, err := f.Forward(ctx, pub.publish)
	require.NoError(t, err)
	require.Positive(t, n)
	require.Less(t, n, 10)
}
