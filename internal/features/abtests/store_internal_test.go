package abtests

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository/repotest"
)

// Spec 004 FR-004: when crypto/rand fails, CreateABTest returns the error and
// writes nothing.
func TestCreateABTest_ReturnsUUIDError(t *testing.T) {
	ctx := context.Background()
	db := repotest.Open(t)
	p, err := db.CreateProject(ctx, "AB", "ab-uuid")
	require.NoError(t, err)

	errNoEntropy := errors.New("no entropy")
	s := NewStore(db.DB(), db.ReadDB())
	s.newID = func() (string, error) { return "", errNoEntropy }

	_, err = s.CreateABTest(ctx, ABTest{ProjectID: p.ID, Name: "t", ConversionEvent: "signup"})
	require.ErrorIs(t, err, errNoEntropy)

	var n int
	require.NoError(t, db.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM ab_tests").Scan(&n))
	require.Zero(t, n)
}
