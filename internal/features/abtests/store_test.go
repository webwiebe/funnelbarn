package abtests_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/features/abtests"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/repotest"
)

func TestStore_CreateABTest(t *testing.T) {
	ctx := context.Background()
	s := repotest.Open(t)
	ab := abtests.NewStore(s.DB(), s.ReadDB())

	p, err := s.CreateProject(ctx, "Proj", "proj-abtest")
	require.NoError(t, err)

	test, err := ab.CreateABTest(ctx, abtests.ABTest{
		ProjectID:       p.ID,
		Name:            "Button Color Test",
		Status:          "running",
		ConversionEvent: "purchase",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, test.ID)
	assert.Equal(t, "Button Color Test", test.Name)
	assert.Equal(t, "running", test.Status)
	assert.Equal(t, "purchase", test.ConversionEvent)
	assert.Equal(t, p.ID, test.ProjectID)
	assert.False(t, test.CreatedAt.IsZero())

	got, err := ab.ABTestByID(ctx, test.ID)
	require.NoError(t, err)
	assert.Equal(t, test.ID, got.ID)
	assert.Equal(t, "Button Color Test", got.Name)
}

func TestStore_ABTestByID_NotFound(t *testing.T) {
	s := repotest.Open(t)
	ab := abtests.NewStore(s.DB(), s.ReadDB())

	_, err := ab.ABTestByID(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestStore_ListABTests(t *testing.T) {
	ctx := context.Background()
	s := repotest.Open(t)
	ab := abtests.NewStore(s.DB(), s.ReadDB())

	p, err := s.CreateProject(ctx, "Proj", "proj-listabtests")
	require.NoError(t, err)

	_, err = ab.CreateABTest(ctx, abtests.ABTest{ProjectID: p.ID, Name: "Test A", Status: "running", ConversionEvent: "click"})
	require.NoError(t, err)
	_, err = ab.CreateABTest(ctx, abtests.ABTest{ProjectID: p.ID, Name: "Test B", Status: "paused", ConversionEvent: "signup"})
	require.NoError(t, err)

	tests, err := ab.ListABTests(ctx, p.ID)
	require.NoError(t, err)
	assert.Len(t, tests, 2)
}

func TestStore_AnalyzeABTest_EmptyHasTwoArms(t *testing.T) {
	ctx := context.Background()
	s := repotest.Open(t)
	ab := abtests.NewStore(s.DB(), s.ReadDB())

	p, err := s.CreateProject(ctx, "Proj", "proj-analyzeab")
	require.NoError(t, err)
	test, err := ab.CreateABTest(ctx, abtests.ABTest{ProjectID: p.ID, Name: "Test", Status: "running", ConversionEvent: "purchase"})
	require.NoError(t, err)

	now := time.Now().UTC()
	results, err := ab.AnalyzeABTest(ctx, test, now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "control", results[0].Variant)
	assert.Equal(t, "variant", results[1].Variant)
}

func TestStore_AnalyzeABTest_RejectsBadFilter(t *testing.T) {
	ctx := context.Background()
	s := repotest.Open(t)
	ab := abtests.NewStore(s.DB(), s.ReadDB())

	now := time.Now().UTC()
	_, err := ab.AnalyzeABTest(ctx, abtests.ABTest{ControlFilter: "{not json"}, now.Add(-time.Hour), now)
	require.Error(t, err)
	_, err = ab.AnalyzeABTest(ctx, abtests.ABTest{VariantFilter: "{not json"}, now.Add(-time.Hour), now)
	require.Error(t, err)
}
