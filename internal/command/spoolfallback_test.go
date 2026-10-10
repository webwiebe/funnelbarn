package command_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
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

	// Drained, the file takes commands again.
	f.Submit(ctx, command.TouchAPIKey{KeyHash: "again"})
	n, err = f.Forward(ctx, pub.publish)
	require.NoError(t, err)
	require.Equal(t, 1, n)
}

// unencodable is a command json.Marshal refuses.
type unencodable struct{ C chan int }

func (unencodable) Kind() string                              { return "unencodable" }
func (unencodable) Project() string                           { return "" }
func (unencodable) Apply(context.Context, command.Deps) error { return nil }

func TestSpoolFallbackDropsWhatItCannotStore(t *testing.T) {
	ctx := context.Background()
	f, err := command.NewSpoolFallback(t.TempDir(), "fb", 0, nil)
	require.NoError(t, err)
	f.Start(ctx)
	f.Submit(ctx, unencodable{C: make(chan int)})
	require.NoError(t, f.Flush(ctx))

	require.NoError(t, f.Close(ctx))
	require.NoError(t, f.Close(ctx), "a second Close is a no-op")
	f.Submit(ctx, command.TouchAPIKey{KeyHash: "late"})

	pub := &recordingPublisher{}
	n, err := f.Forward(ctx, pub.publish)
	require.NoError(t, err)
	require.Zero(t, n)
}

func TestSpoolFallbackCursorFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cursor := filepath.Join(dir, "fb.cursor")
	f, err := command.NewSpoolFallback(dir, "fb", 0, nil)
	require.NoError(t, err)
	defer f.Close(ctx)
	f.Submit(ctx, command.TouchAPIKey{KeyHash: "k1"})
	pub := &recordingPublisher{}

	// A cursor past the end of the file (emptied by hand) starts over.
	require.NoError(t, os.WriteFile(cursor, []byte("999999"), 0o600))
	n, err := f.Forward(ctx, pub.publish)
	require.NoError(t, err)
	require.Zero(t, n)
	n, err = f.Forward(ctx, pub.publish)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// A cursor that does not parse is an error, and nothing is sent.
	f.Submit(ctx, command.TouchAPIKey{KeyHash: "k2"})
	require.NoError(t, os.WriteFile(cursor, []byte("garbage"), 0o600))
	_, err = f.Forward(ctx, pub.publish)
	require.ErrorContains(t, err, "fb.cursor")
	require.Len(t, pub.got, 1)
}

func TestSpoolFallbackNeedsAWritableDir(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	_, err := command.NewSpoolFallback(file, "fb", 0, nil)
	require.Error(t, err)
}

func TestCommandsNeedTheirDependency(t *testing.T) {
	ctx := context.Background()
	for _, c := range []command.Command{
		command.MarkProjectHealth{ProjectID: "p1", Field: command.HealthSetupCalled},
		command.ApplyRecordingChunk{},
		command.IngestRecord{},
	} {
		require.Error(t, c.Apply(ctx, command.Deps{}), c.Kind())
	}

	var calls []string
	deps := command.Deps{
		MarkProjectHealth: func(_ context.Context, projectID, field string) error {
			calls = append(calls, projectID+"/"+field)
			return nil
		},
		ApplyChunk:  func(context.Context, command.ApplyRecordingChunk) error { calls = append(calls, "chunk"); return nil },
		ApplyIngest: func(context.Context, spool.Record) error { calls = append(calls, "ingest"); return nil },
	}
	require.NoError(t, command.MarkProjectHealth{ProjectID: "p1", Field: command.HealthSetupCalled}.Apply(ctx, deps))
	require.NoError(t, command.ApplyRecordingChunk{}.Apply(ctx, deps))
	require.NoError(t, command.IngestRecord{}.Apply(ctx, deps))
	require.Equal(t, []string{"p1/setup_called", "chunk", "ingest"}, calls)
}
