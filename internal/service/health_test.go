package service_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// spyHealthRepo wraps a real store and counts MarkProjectHealthEventsReceived calls.
type spyHealthRepo struct {
	*repository.Store
	markEventsCalls atomic.Int32
}

func (s *spyHealthRepo) MarkProjectHealthEventsReceived(ctx context.Context, projectID string) error {
	s.markEventsCalls.Add(1)
	return s.Store.MarkProjectHealthEventsReceived(ctx, projectID)
}

func TestProjectHealthService_GetProjectHealth(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	svc := service.NewProjectHealthService(store)

	p := createTestProject(ctx, t, store, "health-get", "health-get")

	h, err := svc.GetProjectHealth(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, h.ProjectID)
	assert.False(t, h.SetupCalled)
	assert.False(t, h.EventsReceived)
}

func TestProjectHealthService_MarkSetupCalled(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	svc := service.NewProjectHealthService(store)

	p := createTestProject(ctx, t, store, "health-setup-svc", "health-setup-svc")

	require.NoError(t, svc.MarkSetupCalled(ctx, p.ID))

	h, err := svc.GetProjectHealth(ctx, p.ID)
	require.NoError(t, err)
	assert.True(t, h.SetupCalled)
}

func TestProjectHealthService_MarkEventsReceived(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	svc := service.NewProjectHealthService(store)

	p := createTestProject(ctx, t, store, "health-events-svc", "health-events-svc")

	require.NoError(t, svc.MarkEventsReceived(ctx, p.ID))

	h, err := svc.GetProjectHealth(ctx, p.ID)
	require.NoError(t, err)
	assert.True(t, h.EventsReceived)
}

func TestProjectHealthService_MarkFlagsEvaluated(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	svc := service.NewProjectHealthService(store)

	p := createTestProject(ctx, t, store, "health-flags-svc", "health-flags-svc")

	require.NoError(t, svc.MarkFlagsEvaluated(ctx, p.ID))

	h, err := svc.GetProjectHealth(ctx, p.ID)
	require.NoError(t, err)
	assert.True(t, h.FlagsEvaluated)
}

func TestProjectHealthService_MarkRecordingsReceived(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	svc := service.NewProjectHealthService(store)

	p := createTestProject(ctx, t, store, "health-rec-svc", "health-rec-svc")

	require.NoError(t, svc.MarkRecordingsReceived(ctx, p.ID))

	h, err := svc.GetProjectHealth(ctx, p.ID)
	require.NoError(t, err)
	assert.True(t, h.RecordingsReceived)
}

func TestProjectHealthService_ResetProjectHealth(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	svc := service.NewProjectHealthService(store)

	p := createTestProject(ctx, t, store, "health-reset-svc", "health-reset-svc")

	require.NoError(t, svc.MarkSetupCalled(ctx, p.ID))
	require.NoError(t, svc.MarkEventsReceived(ctx, p.ID))

	require.NoError(t, svc.ResetProjectHealth(ctx, p.ID))

	h, err := svc.GetProjectHealth(ctx, p.ID)
	require.NoError(t, err)
	assert.False(t, h.SetupCalled)
	assert.False(t, h.EventsReceived)
}

// TestProjectHealthService_CacheSkipsDBWrite verifies that a second Mark* call is a
// no-op once the field is confirmed true: the DB write method must not be called again.
func TestProjectHealthService_CacheSkipsDBWrite(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	spy := &spyHealthRepo{Store: store}
	svc := service.NewProjectHealthService(spy)

	p := createTestProject(ctx, t, store, "health-cache-skip", "health-cache-skip")

	// First call: field is false → should write to DB (1 call).
	require.NoError(t, svc.MarkEventsReceived(ctx, p.ID))
	assert.Equal(t, int32(1), spy.markEventsCalls.Load())

	// Second call: cache has field=true → should skip DB entirely (still 1 call).
	require.NoError(t, svc.MarkEventsReceived(ctx, p.ID))
	assert.Equal(t, int32(1), spy.markEventsCalls.Load())
}

// TestProjectHealthService_CacheEvictedOnReset verifies that after Reset, the next
// Mark* call re-reads from DB and writes again if the field is false.
func TestProjectHealthService_CacheEvictedOnReset(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	spy := &spyHealthRepo{Store: store}
	svc := service.NewProjectHealthService(spy)

	p := createTestProject(ctx, t, store, "health-cache-evict", "health-cache-evict")

	require.NoError(t, svc.MarkEventsReceived(ctx, p.ID))
	assert.Equal(t, int32(1), spy.markEventsCalls.Load())

	require.NoError(t, svc.ResetProjectHealth(ctx, p.ID))

	// After reset the cache is evicted; the next Mark* must write to DB again.
	require.NoError(t, svc.MarkEventsReceived(ctx, p.ID))
	assert.Equal(t, int32(2), spy.markEventsCalls.Load())
}

// createTestProject is a helper that creates a project directly via the store.
func createTestProject(ctx context.Context, t *testing.T, store *repository.Store, name, slug string) repository.Project {
	t.Helper()
	projSvc := service.NewProjectService(store)
	p, err := projSvc.CreateProject(ctx, name, slug)
	require.NoError(t, err)
	return p
}
