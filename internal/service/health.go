package service

import (
	"context"
	"log/slog"
	"sync"

	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// projectHealthService implements ProjectHealth with an in-memory cache that
// short-circuits all DB interaction once a feature is confirmed true.
//
// Each Mark* call follows this sequence:
//  1. Read-lock cache — if the field is already true, return immediately (zero DB calls).
//  2. Load the current row from DB (lazy per project per restart).
//  3. If DB already has the field true, update cache and return.
//  4. Otherwise write to DB, then update cache.
//
// This guarantees at most one DB write per project feature across the server lifetime,
// and zero writes after the field is confirmed true.
type projectHealthService struct {
	repo  ports.ProjectHealthRepo
	mu    sync.RWMutex
	cache map[string]repository.ProjectHealth // keyed by project_id
}

// NewProjectHealthService creates a new projectHealthService.
func NewProjectHealthService(repo ports.ProjectHealthRepo) *projectHealthService {
	return &projectHealthService{
		repo:  repo,
		cache: make(map[string]repository.ProjectHealth),
	}
}

// GetProjectHealth returns the current health status directly from DB (read path, infrequent).
func (s *projectHealthService) GetProjectHealth(ctx context.Context, projectID string) (repository.ProjectHealth, error) {
	return s.repo.GetProjectHealth(ctx, projectID)
}

// ResetProjectHealth zeros all health flags and evicts the project from the cache.
func (s *projectHealthService) ResetProjectHealth(ctx context.Context, projectID string) error {
	if err := s.repo.ResetProjectHealth(ctx, projectID); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.cache, projectID)
	s.mu.Unlock()
	return nil
}

func (s *projectHealthService) MarkSetupCalled(ctx context.Context, projectID string) error {
	_, err := s.markField(ctx, projectID,
		func(h repository.ProjectHealth) bool { return h.SetupCalled },
		s.repo.MarkProjectHealthSetupCalled,
		func(h *repository.ProjectHealth) { h.SetupCalled = true },
	)
	return err
}

// MarkEventsReceived records that at least one event was ingested for
// projectID. See the ProjectHealth interface doc for the meaning of the
// returned bool.
func (s *projectHealthService) MarkEventsReceived(ctx context.Context, projectID string) (bool, error) {
	return s.markField(ctx, projectID,
		func(h repository.ProjectHealth) bool { return h.EventsReceived },
		s.repo.MarkProjectHealthEventsReceived,
		func(h *repository.ProjectHealth) { h.EventsReceived = true },
	)
}

func (s *projectHealthService) MarkFlagsEvaluated(ctx context.Context, projectID string) error {
	_, err := s.markField(ctx, projectID,
		func(h repository.ProjectHealth) bool { return h.FlagsEvaluated },
		s.repo.MarkProjectHealthFlagsEvaluated,
		func(h *repository.ProjectHealth) { h.FlagsEvaluated = true },
	)
	return err
}

func (s *projectHealthService) MarkRecordingsReceived(ctx context.Context, projectID string) error {
	_, err := s.markField(ctx, projectID,
		func(h repository.ProjectHealth) bool { return h.RecordingsReceived },
		s.repo.MarkProjectHealthRecordingsReceived,
		func(h *repository.ProjectHealth) { h.RecordingsReceived = true },
	)
	return err
}

// markField is the shared fast-path logic for all Mark* methods. The
// returned bool is true only when this call is the one that flipped the
// field from false to true (in the DB, not merely in this process's cache).
func (s *projectHealthService) markField(
	ctx context.Context,
	projectID string,
	isSet func(repository.ProjectHealth) bool,
	dbMark func(context.Context, string) error,
	cacheSet func(*repository.ProjectHealth),
) (bool, error) {
	// Fast path: field already true in cache — skip all DB interaction.
	s.mu.RLock()
	cached, ok := s.cache[projectID]
	s.mu.RUnlock()
	if ok && isSet(cached) {
		return false, nil
	}

	// Slow path: cache miss or field is false. Load from DB to confirm current state.
	current, err := s.repo.GetProjectHealth(ctx, projectID)
	if err != nil {
		slog.WarnContext(ctx, "project health: get", "project_id", projectID, "err", err)
		return false, err
	}

	s.mu.Lock()
	s.cache[projectID] = current
	s.mu.Unlock()

	if isSet(current) {
		// DB already has it true — cache is now warm, nothing to write.
		return false, nil
	}

	// Field is false in DB — write the transition.
	if err := dbMark(ctx, projectID); err != nil {
		return false, err
	}

	// Update cache to reflect the new state.
	s.mu.Lock()
	updated := s.cache[projectID]
	cacheSet(&updated)
	s.cache[projectID] = updated
	s.mu.Unlock()

	return true, nil
}
