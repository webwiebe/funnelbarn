package abtests_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/features/abtests"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/repotest"
)

func newService(t *testing.T) (*abtests.Service, string) {
	t.Helper()
	s := repotest.Open(t)
	p, err := s.CreateProject(context.Background(), "AB Project", "ab-project")
	require.NoError(t, err)
	return abtests.New(s.DB(), s.ReadDB()), p.ID
}

func TestService_CreateABTest_Valid(t *testing.T) {
	svc, pid := newService(t)

	test, err := svc.CreateABTest(context.Background(), abtests.ABTest{
		ProjectID: pid, Name: "My Test", Status: "running", ConversionEvent: "purchase",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, test.ID)
	assert.Equal(t, "My Test", test.Name)
	assert.Equal(t, "purchase", test.ConversionEvent)
	assert.Equal(t, pid, test.ProjectID)
}

func TestService_CreateABTest_Validation(t *testing.T) {
	svc, pid := newService(t)
	cases := map[string]abtests.ABTest{
		"empty project":          {Name: "n", ConversionEvent: "c"},
		"empty name":             {ProjectID: pid, ConversionEvent: "c"},
		"whitespace name":        {ProjectID: pid, Name: "  ", ConversionEvent: "c"},
		"empty conversion event": {ProjectID: pid, Name: "n"},
		"invalid status":         {ProjectID: pid, Name: "n", Status: "launched", ConversionEvent: "c"},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := svc.CreateABTest(context.Background(), in)
			require.Error(t, err)
			require.True(t, domain.IsValidation(err), "expected validation error, got: %v", err)
		})
	}
}

func TestService_CreateABTest_ValidStatuses(t *testing.T) {
	svc, pid := newService(t)
	for i, status := range []string{"running", "paused", "completed"} {
		_, err := svc.CreateABTest(context.Background(), abtests.ABTest{
			ProjectID: pid, Name: fmt.Sprintf("Test %d", i), Status: status, ConversionEvent: "purchase",
		})
		require.NoError(t, err, "status %q should be valid", status)
	}
}

func TestService_ListGetAnalyze(t *testing.T) {
	ctx := context.Background()
	svc, pid := newService(t)

	created, err := svc.CreateABTest(ctx, abtests.ABTest{ProjectID: pid, Name: "Button Colour", Status: "running", ConversionEvent: "purchase"})
	require.NoError(t, err)

	tests, err := svc.ListABTests(ctx, pid)
	require.NoError(t, err)
	require.Len(t, tests, 1)

	got, err := svc.GetABTest(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)

	_, err = svc.GetABTest(ctx, "missing")
	require.ErrorIs(t, err, domain.ErrNotFound)

	results, err := svc.AnalyzeABTest(ctx, got, time.Now().Add(-24*time.Hour), time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.Len(t, results, 2)
}
