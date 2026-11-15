package widgets

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository/repotest"
)

// Spec 004 FR-004: when crypto/rand fails, CreateWidget returns the error and
// writes nothing.
func TestCreateWidget_ReturnsUUIDError(t *testing.T) {
	ctx := context.Background()
	db := repotest.Open(t)
	p, err := db.CreateProject(ctx, "Widgets", "widgets-uuid")
	require.NoError(t, err)

	errNoEntropy := errors.New("no entropy")
	s := NewStore(db.DB(), db.ReadDB())
	s.newID = func() (string, error) { return "", errNoEntropy }

	_, err = s.CreateWidget(ctx, Widget{ProjectID: p.ID, EventName: "signup"})
	require.ErrorIs(t, err, errNoEntropy)

	var n int
	require.NoError(t, db.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM dashboard_widgets").Scan(&n))
	require.Zero(t, n)
}
