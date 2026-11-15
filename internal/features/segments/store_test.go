package segments

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/repotest"
)

func TestStore_Segments_CRUD(t *testing.T) {
	ctx := context.Background()
	db := repotest.Open(t)
	s := NewStore(db.DB(), db.ReadDB())

	p, err := db.CreateProject(ctx, "Seg Project", "seg-project")
	require.NoError(t, err)

	rules := []domain.SegmentRule{
		{Field: "country_code", Operator: "eq", Value: "NL"},
		{Field: "device_type", Operator: "eq", Value: "mobile"},
	}

	// Create
	seg, err := s.CreateSegment(ctx, Segment{
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
	seg.Rules = []domain.SegmentRule{{Field: "country_code", Operator: "eq", Value: "NL"}}
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
	db := repotest.Open(t)
	s := NewStore(db.DB(), db.ReadDB())

	p, err := db.CreateProject(ctx, "Empty", "empty-seg")
	require.NoError(t, err)

	list, err := s.ListSegments(ctx, p.ID)
	require.NoError(t, err)
	assert.Empty(t, list)
}
