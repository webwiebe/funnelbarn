package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestStripRecordingPageURLs_DryRunWritesNothingApplyStripsOneProject(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	a, err := s.CreateProject(ctx, "Strip A", "strip-a")
	require.NoError(t, err)
	b, err := s.CreateProject(ctx, "Strip B", "strip-b")
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	seed := map[string]struct{ project, url string }{
		"r1": {a.ID, "https://iam.example.com/auth/login?redirect_uri=%2Foauth2&state=abc#frag"},
		"r2": {a.ID, "https://iam.example.com/auth/signup"},
		"r3": {a.ID, "https://iam.example.com/auth/login?utm_source=mail&state=abc"},
		"r4": {a.ID, "https://iam.example.com/auth/login?utm_source=mail"},
		"r5": {b.ID, "https://other.example.com/page?q=1"},
	}
	for id, v := range seed {
		rec := makeRecording(v.project, "sess-"+id, now)
		rec.ID = id
		rec.PageURL = v.url
		require.NoError(t, s.UpsertRecording(ctx, rec))
	}

	dry, err := s.StripRecordingPageURLs(ctx, a.ID, 2, false)
	require.NoError(t, err)
	assert.Equal(t, repository.PageURLStrip{Scanned: 3, Changed: 2, Batches: 2}, dry)
	got, err := s.GetRecording(ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, seed["r1"].url, got.PageURL, "a dry run changes nothing")

	applied, err := s.StripRecordingPageURLs(ctx, a.ID, 2, true)
	require.NoError(t, err)
	assert.Equal(t, dry, applied)

	want := map[string]string{
		"r1": "https://iam.example.com/auth/login",
		"r2": "https://iam.example.com/auth/signup",
		"r3": "https://iam.example.com/auth/login?utm_source=mail",
		"r4": "https://iam.example.com/auth/login?utm_source=mail",
		"r5": "https://other.example.com/page?q=1",
	}
	for id, url := range want {
		rec, err := s.GetRecording(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, url, rec.PageURL, id)
	}

	again, err := s.StripRecordingPageURLs(ctx, a.ID, 2, true)
	require.NoError(t, err)
	assert.Zero(t, again.Changed, "a second run finds nothing to change")
}

func TestStripRecordingPageURLs_CapsTheBatch(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Strip Cap", "strip-cap")
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Second)
	total := repository.MaxPageURLBatch + 3
	for i := 0; i < total; i++ {
		rec := makeRecording(p.ID, fmt.Sprintf("s%d", i), now)
		rec.ID = fmt.Sprintf("rec-%04d", i)
		rec.PageURL = "https://x.example.com/a?state=1"
		require.NoError(t, s.UpsertRecording(ctx, rec))
	}
	got, err := s.StripRecordingPageURLs(ctx, p.ID, 100000, true)
	require.NoError(t, err)
	assert.Equal(t, total, got.Changed)
	assert.Equal(t, 2, got.Batches, "a huge batch flag is capped in code")
}
