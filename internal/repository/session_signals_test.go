package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

func TestStore_UpsertSessionSignals(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	p, err := s.CreateProject(ctx, "Signals", "signals")
	require.NoError(t, err)

	sess := domain.Session{
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

	signals := domain.SessionSignals{
		ScreenWidth:     &w,
		ScreenHeight:    &h,
		PixelRatio:      &ratio,
		Touch:           &touch,
		DarkMode:        &dark,
		ReducedMotion:   &reduced,
		BrowserTimezone: "Europe/Amsterdam",
		CPUCores:        &cores,
	}
	require.NoError(t, s.UpsertSessionSignals(ctx, p.ID, "sess-123", signals))

	// Second upsert should be a no-op (signals_collected = 1 guard).
	w2 := 800
	signals2 := domain.SessionSignals{ScreenWidth: &w2}
	require.NoError(t, s.UpsertSessionSignals(ctx, p.ID, "sess-123", signals2))
}
