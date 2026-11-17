package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func backfillEvent(t *testing.T, s *repository.Store, projectID, session, env string) {
	t.Helper()
	e := insertEvent(t, s, projectID, session, "page_view", "https://example.com/")
	_, err := s.DB().ExecContext(context.Background(), `UPDATE events SET environment = ? WHERE id = ?`, env, e.ID)
	require.NoError(t, err)
}

func backfillRecording(t *testing.T, s *repository.Store, projectID, id, session, env string) {
	t.Helper()
	rec := makeRecording(projectID, session, time.Now().UTC().Truncate(time.Second))
	rec.ID = id
	rec.Environment = env
	require.NoError(t, s.UpsertRecording(context.Background(), rec))
}

func TestBackfillRecordingEnvironments(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	a, err := s.CreateProject(ctx, "Backfill A", "backfill-a")
	require.NoError(t, err)
	b, err := s.CreateProject(ctx, "Backfill B", "backfill-b")
	require.NoError(t, err)

	backfillEvent(t, s, a.ID, "sess-test", "test")
	backfillEvent(t, s, a.ID, "sess-test", "test")
	backfillEvent(t, s, a.ID, "sess-mixed", "test")
	backfillEvent(t, s, a.ID, "sess-mixed", "production")
	backfillEvent(t, s, a.ID, "sess-untagged", "")
	backfillEvent(t, s, b.ID, "sess-other", "staging")

	backfillRecording(t, s, a.ID, "r-test", "sess-test", "")
	backfillRecording(t, s, a.ID, "r-mixed", "sess-mixed", "")
	backfillRecording(t, s, a.ID, "r-none", "sess-none", "")
	backfillRecording(t, s, a.ID, "r-untagged", "sess-untagged", "")
	backfillRecording(t, s, a.ID, "r-set", "sess-test", "staging")
	backfillRecording(t, s, b.ID, "r-other", "sess-other", "")

	dry, err := s.BackfillRecordingEnvironments(ctx, a.ID, 2, false)
	require.NoError(t, err)
	assert.Equal(t, repository.EnvironmentBackfill{Scanned: 4, Filled: 2, Mixed: 1, NoEvents: 1, Batches: 2}, dry)
	got, err := s.GetRecording(ctx, "r-test")
	require.NoError(t, err)
	assert.Equal(t, "", got.Environment, "a dry run writes nothing")

	applied, err := s.BackfillRecordingEnvironments(ctx, a.ID, 2, true)
	require.NoError(t, err)
	assert.Equal(t, dry, applied)

	want := map[string]string{
		"r-test": "test", "r-untagged": "production", "r-mixed": "", "r-none": "",
		"r-set": "staging", "r-other": "",
	}
	for id, env := range want {
		rec, err := s.GetRecording(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, env, rec.Environment, id)
	}

	again, err := s.BackfillRecordingEnvironments(ctx, a.ID, 2, true)
	require.NoError(t, err)
	assert.Equal(t, 0, again.Filled, "a second run fills nothing")
	assert.Equal(t, 2, again.Scanned, "mixed and event-less recordings stay and are counted again")
}
