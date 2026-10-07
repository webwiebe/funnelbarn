package repository_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// TestPurgeStopsOnCancelledContext checks that the batched purge honours ctx
// between batches: a cancelled context deletes nothing and reports the error.
func TestPurgeStopsOnCancelledContext(t *testing.T) {
	ctx := context.Background()
	s, err := repository.Open(filepath.Join(t.TempDir(), "purge-cancel.db"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	p, err := s.CreateProject(ctx, "p", "p")
	require.NoError(t, err)
	old := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err = s.DB().ExecContext(ctx,
		`INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at) VALUES ('e1', ?, 's1', 'page_view', 'i1', ?)`,
		p.ID, old)
	require.NoError(t, err)

	cctx, cancel := context.WithCancel(ctx)
	cancel()
	n, err := s.PurgeOldEvents(cctx, time.Now())
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, n)

	n, err = s.PurgeOldEvents(ctx, time.Now())
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
}
