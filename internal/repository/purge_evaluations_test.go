package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestStore_PurgeOldEvaluations(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "PurgeEval", "purge-eval")
	require.NoError(t, err)

	f, err := s.CreateFlag(ctx, repository.FeatureFlag{
		ProjectID:      p.ID,
		FlagKey:        "test-flag",
		Name:           "Test Flag",
		FlagType:       "boolean",
		Variants:       `{"on":true,"off":false}`,
		DefaultVariant: "off",
		Split:          `{"on":50,"off":50}`,
		Status:         "active",
	})
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		err = s.RecordEvaluation(ctx, repository.FlagEvaluation{
			FlagID:      f.ID,
			ProjectID:   p.ID,
			Variant:     "on",
			ContextHash: "hash",
		})
		require.NoError(t, err)
	}

	// Purge with future cutoff — should remove all.
	n, err := s.PurgeOldEvaluations(ctx, time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(5), n)

	// Purge again — should remove 0.
	n2, err := s.PurgeOldEvaluations(ctx, time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(0), n2)
}
