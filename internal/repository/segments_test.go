package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestStore_Segments_CRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Seg Project", "seg-project")
	require.NoError(t, err)

	rules := []repository.SegmentRule{
		{Field: "country_code", Operator: "eq", Value: "NL"},
		{Field: "device_type", Operator: "eq", Value: "mobile"},
	}

	// Create
	seg, err := s.CreateSegment(ctx, repository.Segment{
		ProjectID: p.ID,
		Name:      "Dutch mobile",
		Rules:     rules,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, seg.ID)
	assert.Equal(t, "Dutch mobile", seg.Name)
	assert.Len(t, seg.Rules, 2)

	// SegmentByID
	got, err := s.SegmentByID(ctx, seg.ID)
	require.NoError(t, err)
	assert.Equal(t, seg.ID, got.ID)
	assert.Equal(t, "NL", got.Rules[0].Value)

	// ListSegments
	list, err := s.ListSegments(ctx, p.ID)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, seg.ID, list[0].ID)

	// UpdateSegment
	seg.Name = "Dutch desktop"
	seg.Rules = []repository.SegmentRule{{Field: "country_code", Operator: "eq", Value: "NL"}}
	updated, err := s.UpdateSegment(ctx, seg)
	require.NoError(t, err)
	assert.Equal(t, "Dutch desktop", updated.Name)
	assert.Len(t, updated.Rules, 1)

	// DeleteSegment
	err = s.DeleteSegment(ctx, seg.ID)
	require.NoError(t, err)

	_, err = s.SegmentByID(ctx, seg.ID)
	require.Error(t, err)
}

func TestStore_ListSegments_Empty(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Empty", "empty-seg")
	require.NoError(t, err)

	list, err := s.ListSegments(ctx, p.ID)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestStore_UpsertSessionSignals(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Signals", "signals")
	require.NoError(t, err)

	sess := repository.Session{
		ID:          "sess-123",
		ProjectID:   p.ID,
		FirstSeenAt: time.Now().UTC(),
		LastSeenAt:  time.Now().UTC(),
	}
	require.NoError(t, s.UpsertSession(ctx, sess))

	w, h := 1920, 1080
	ratio := 2.0
	touch := false
	dark := true
	reduced := false
	cores := 8

	signals := repository.SessionSignals{
		ScreenWidth:     &w,
		ScreenHeight:    &h,
		PixelRatio:      &ratio,
		Touch:           &touch,
		DarkMode:        &dark,
		ReducedMotion:   &reduced,
		BrowserTimezone: "Europe/Amsterdam",
		CPUCores:        &cores,
	}
	require.NoError(t, s.UpsertSessionSignals(ctx, "sess-123", signals))

	// Second upsert should be a no-op (signals_collected = 1 guard).
	w2 := 800
	signals2 := repository.SessionSignals{ScreenWidth: &w2}
	require.NoError(t, s.UpsertSessionSignals(ctx, "sess-123", signals2))
}
