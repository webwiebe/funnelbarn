package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func autoFlag(projectID, key string) repository.FeatureFlag {
	return repository.FeatureFlag{
		ProjectID:      projectID,
		FlagKey:        key,
		Name:           key,
		FlagType:       "number",
		Variants:       `{"default":3}`,
		DefaultVariant: "default",
		Split:          "{}",
		TargetingRules: "[]",
		Status:         "inactive",
		Origin:         "auto",
	}
}

func TestStore_EnsureAutoFlag_Idempotent(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "AutoFlag", "auto-flag")
	require.NoError(t, err)

	f1, err := s.EnsureAutoFlag(ctx, autoFlag(p.ID, "anon_qr_limit"))
	require.NoError(t, err)
	require.Equal(t, "auto", f1.Origin)
	require.Equal(t, "inactive", f1.Status)

	// Same key again — must return the existing row, not a duplicate.
	f2, err := s.EnsureAutoFlag(ctx, autoFlag(p.ID, "anon_qr_limit"))
	require.NoError(t, err)
	require.Equal(t, f1.ID, f2.ID)

	flags, err := s.ListFlags(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, flags, 1)
}

func TestStore_CountAutoFlags_IgnoresManual(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "CountAuto", "count-auto")
	require.NoError(t, err)

	_, err = s.EnsureAutoFlag(ctx, autoFlag(p.ID, "a1"))
	require.NoError(t, err)
	_, err = s.EnsureAutoFlag(ctx, autoFlag(p.ID, "a2"))
	require.NoError(t, err)
	// A manual flag must not count.
	_, err = s.CreateFlag(ctx, repository.FeatureFlag{
		ProjectID: p.ID, FlagKey: "manual", Name: "manual", FlagType: "boolean",
		Variants: `{"on":true,"off":false}`, DefaultVariant: "off", Split: "{}", Status: "active",
	})
	require.NoError(t, err)

	n, err := s.CountAutoFlags(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestStore_TouchFlagEvaluated(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Touch", "touch")
	require.NoError(t, err)

	f, err := s.EnsureAutoFlag(ctx, autoFlag(p.ID, "k"))
	require.NoError(t, err)
	require.Nil(t, f.LastEvaluatedAt)

	require.NoError(t, s.TouchFlagEvaluated(ctx, f.ID))

	got, err := s.FlagByKey(ctx, p.ID, "k")
	require.NoError(t, err)
	require.NotNil(t, got.LastEvaluatedAt, "last_evaluated_at should be set after touch")
}

func TestStore_PurgeStaleAutoFlags(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "PurgeAuto", "purge-auto")
	require.NoError(t, err)

	// Stale auto+inactive — should be pruned.
	_, err = s.EnsureAutoFlag(ctx, autoFlag(p.ID, "stale_auto"))
	require.NoError(t, err)

	// auto+active — kept (configured/activated).
	activeAuto := autoFlag(p.ID, "active_auto")
	activeAuto.Status = "active"
	_, err = s.EnsureAutoFlag(ctx, activeAuto)
	require.NoError(t, err)

	// manual+inactive — kept (human-claimed).
	_, err = s.CreateFlag(ctx, repository.FeatureFlag{
		ProjectID: p.ID, FlagKey: "manual_inactive", Name: "m", FlagType: "boolean",
		Variants: `{"on":true,"off":false}`, DefaultVariant: "off", Split: "{}", Status: "inactive",
	})
	require.NoError(t, err)

	// Cutoff well in the future (wide enough to dwarf any DB/driver timezone
	// skew): everything was created before it, so only the stale auto+inactive
	// flag qualifies — status and origin filters protect the other two.
	n, err := s.PurgeStaleAutoFlags(ctx, time.Now().Add(48*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	flags, err := s.ListFlags(ctx, p.ID)
	require.NoError(t, err)
	assert.Len(t, flags, 2)
	_, err = s.FlagByKey(ctx, p.ID, "stale_auto")
	assert.Error(t, err, "stale auto flag should be gone")
}

func TestStore_PurgeStaleAutoFlags_KeepsRecentlyEvaluated(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "PurgeRecent", "purge-recent")
	require.NoError(t, err)

	f, err := s.EnsureAutoFlag(ctx, autoFlag(p.ID, "live_auto"))
	require.NoError(t, err)
	require.NoError(t, s.TouchFlagEvaluated(ctx, f.ID)) // last_evaluated_at = now

	// Cutoff well in the past (wide enough to dwarf timezone skew): a flag last
	// evaluated "now" is far newer than the cutoff, so it is kept.
	n, err := s.PurgeStaleAutoFlags(ctx, time.Now().Add(-48*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}
