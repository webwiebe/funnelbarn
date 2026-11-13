package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// Pins the parts of SessionsAtStep the environment test leaves open: the
// ascending order, the limit, the range check and a three-step funnel.
func TestSessionsAtStep_OrderLimitAndRange(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Drill", "drill-limit")
	require.NoError(t, err)

	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	// c, a, b viewed; b also added to cart; only a reached checkout.
	for _, sess := range []string{"c", "a", "b"} {
		envEvent(t, s, p.ID, sess, "", "production", "view", "Chrome", at)
	}
	envEvent(t, s, p.ID, "b", "", "production", "cart", "Chrome", at.Add(time.Minute))
	envEvent(t, s, p.ID, "a", "", "production", "cart", "Chrome", at.Add(time.Minute))
	envEvent(t, s, p.ID, "a", "", "production", "checkout", "Chrome", at.Add(2*time.Minute))

	f, err := s.CreateFunnel(ctx, repository.Funnel{ProjectID: p.ID, Name: "Shop",
		Steps: []repository.FunnelStep{{EventName: "view"}, {EventName: "cart"}, {EventName: "checkout"}}})
	require.NoError(t, err)
	from, to := at.Add(-time.Hour), at.Add(time.Hour)

	got, err := s.SessionsAtStep(ctx, f, 1, from, to, 100)
	require.NoError(t, err)
	require.Equal(t, []string{"c"}, got, "step 1 drop-offs: viewed but never added to cart")

	got, err = s.SessionsAtStep(ctx, f, 2, from, to, 100)
	require.NoError(t, err)
	require.Equal(t, []string{"b"}, got, "step 2 drop-offs: cart without checkout")

	got, err = s.SessionsAtStep(ctx, f, 3, from, to, 100)
	require.NoError(t, err)
	require.Equal(t, []string{"a"}, got, "the last step lists converters")

	// Limit keeps the ascending head; zero falls back to the default of 100.
	f2, err := s.CreateFunnel(ctx, repository.Funnel{ProjectID: p.ID, Name: "Views",
		Steps: []repository.FunnelStep{{EventName: "view"}}})
	require.NoError(t, err)
	got, err = s.SessionsAtStep(ctx, f2, 1, from, to, 2)
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, got)
	got, err = s.SessionsAtStep(ctx, f2, 1, from, to, 0)
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c"}, got)

	for _, step := range []int{0, 4} {
		_, err = s.SessionsAtStep(ctx, f, step, from, to, 100)
		require.ErrorContains(t, err, "out of range", "step %d", step)
	}
}
