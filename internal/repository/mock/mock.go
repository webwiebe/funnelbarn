// Package mock provides an in-memory implementation of repository.Querier for testing.
package mock

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// compile-time check that *Store satisfies repository.Querier.
var _ repository.Querier = (*Store)(nil)

var mockIDCounter int64

func newMockID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, atomic.AddInt64(&mockIDCounter, 1))
}

// Store is a thread-safe in-memory implementation of repository.Querier.
type Store struct {
	mu       sync.RWMutex
	projects map[string]repository.Project
	apikeys  map[string]repository.APIKey // keyed by ID
	users    map[string]repository.User   // keyed by username
	funnels  map[string]repository.Funnel
	abtests  map[string]repository.ABTest
	sessions map[string]repository.Session
	events   []repository.Event
}

// New returns a fresh empty Store.
func New() *Store {
	return &Store{
		projects: make(map[string]repository.Project),
		apikeys:  make(map[string]repository.APIKey),
		users:    make(map[string]repository.User),
		funnels:  make(map[string]repository.Funnel),
		abtests:  make(map[string]repository.ABTest),
		sessions: make(map[string]repository.Session),
	}
}

// ── Projects ──────────────────────────────────────────────────────────────────

func (s *Store) CreateProject(ctx context.Context, name, slug string) (repository.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range s.projects {
		if p.Slug == slug {
			return repository.Project{}, fmt.Errorf("UNIQUE constraint failed: projects.slug: %w", domain.ErrConflict)
		}
	}

	p := repository.Project{
		ID:        newMockID("proj"),
		Name:      name,
		Slug:      slug,
		Status:    "active",
		CreatedAt: time.Now(),
	}
	s.projects[p.ID] = p
	return p, nil
}

func (s *Store) ProjectByID(ctx context.Context, id string) (repository.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.projects[id]
	if !ok {
		return repository.Project{}, sql.ErrNoRows
	}
	return p, nil
}

func (s *Store) ProjectBySlug(ctx context.Context, slug string) (repository.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.projects {
		if p.Slug == slug {
			return p, nil
		}
	}
	return repository.Project{}, sql.ErrNoRows
}

func (s *Store) ListProjects(ctx context.Context) ([]repository.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]repository.Project, 0, len(s.projects))
	for _, p := range s.projects {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Store) UpdateProject(ctx context.Context, id, name, domain string) (repository.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[id]
	if !ok {
		return repository.Project{}, sql.ErrNoRows
	}
	p.Name = name
	p.Domain = domain
	s.projects[id] = p
	return p, nil
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.projects, id)
	return nil
}

func (s *Store) ApproveProject(ctx context.Context, id string) (repository.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[id]
	if !ok {
		return repository.Project{}, sql.ErrNoRows
	}
	p.Status = "active"
	s.projects[id] = p
	return p, nil
}

func (s *Store) EnsureProject(ctx context.Context, slug string) (repository.Project, error) {
	p, err := s.ProjectBySlug(ctx, slug)
	if err == nil {
		return p, nil
	}
	if err != sql.ErrNoRows {
		return repository.Project{}, err
	}
	return s.CreateProject(ctx, slug, slug)
}

func (s *Store) EnsureProjectPending(ctx context.Context, name, slug string) (repository.Project, error) {
	p, err := s.ProjectBySlug(ctx, slug)
	if err == nil {
		return p, nil
	}
	if err != sql.ErrNoRows {
		return repository.Project{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	proj := repository.Project{
		ID:        newMockID("proj"),
		Name:      name,
		Slug:      slug,
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	s.projects[proj.ID] = proj
	return proj, nil
}

func (s *Store) HasProjects(ctx context.Context) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.projects) > 0, nil
}

func (s *Store) EnsureSetupAPIKey(ctx context.Context, projectID, keySHA256 string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Idempotent: if a key with the same hash already exists, do nothing.
	for _, k := range s.apikeys {
		if k.KeyHash == keySHA256 {
			return nil
		}
	}
	id := newMockID("apikey")
	s.apikeys[id] = repository.APIKey{
		ID:        id,
		ProjectID: projectID,
		Name:      "setup",
		KeyHash:   keySHA256,
		Scope:     "ingest",
		CreatedAt: time.Now(),
	}
	return nil
}

// ── Users ─────────────────────────────────────────────────────────────────────

func (s *Store) UpsertUser(ctx context.Context, username, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[username]
	if !ok {
		u = repository.User{
			ID:        newMockID("user"),
			Username:  username,
			CreatedAt: time.Now(),
		}
	}
	u.PasswordHash = passwordHash
	s.users[username] = u
	return nil
}

func (s *Store) UserByUsername(ctx context.Context, username string) (repository.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[username]
	if !ok {
		return repository.User{}, sql.ErrNoRows
	}
	return u, nil
}

// ── API Keys ──────────────────────────────────────────────────────────────────

func (s *Store) CreateAPIKey(ctx context.Context, name, projectID, keySHA256, scope string) (repository.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := newMockID("apikey")
	k := repository.APIKey{
		ID:        id,
		ProjectID: projectID,
		Name:      name,
		KeyHash:   keySHA256,
		Scope:     scope,
		CreatedAt: time.Now(),
	}
	s.apikeys[id] = k
	return k, nil
}

func (s *Store) ListAPIKeys(ctx context.Context, projectID string) ([]repository.APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []repository.APIKey
	for _, k := range s.apikeys {
		if k.ProjectID == projectID {
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) ListAllAPIKeys(ctx context.Context) ([]repository.APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]repository.APIKey, 0, len(s.apikeys))
	for _, k := range s.apikeys {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) DeleteAPIKey(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.apikeys, id)
	return nil
}

func (s *Store) ValidAPIKeySHA256(ctx context.Context, keySHA256 string) (projectID string, scope string, found bool, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, k := range s.apikeys {
		if k.KeyHash == keySHA256 {
			return k.ProjectID, k.Scope, true, nil
		}
	}
	return "", "", false, nil
}

func (s *Store) TouchAPIKey(ctx context.Context, keySHA256 string) error {
	// No-op: mock doesn't track last_used_at.
	return nil
}

// ── Funnels ───────────────────────────────────────────────────────────────────

func (s *Store) CreateFunnel(ctx context.Context, f repository.Funnel) (repository.Funnel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f.ID = newMockID("funnel")
	f.CreatedAt = time.Now()
	for i := range f.Steps {
		f.Steps[i].ID = newMockID("step")
		f.Steps[i].FunnelID = f.ID
		f.Steps[i].StepOrder = i + 1
	}
	s.funnels[f.ID] = f
	return f, nil
}

func (s *Store) FunnelByID(ctx context.Context, id string) (repository.Funnel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.funnels[id]
	if !ok {
		return repository.Funnel{}, sql.ErrNoRows
	}
	return f, nil
}

func (s *Store) ListFunnels(ctx context.Context, projectID string) ([]repository.Funnel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []repository.Funnel
	for _, f := range s.funnels {
		if f.ProjectID == projectID {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) UpdateFunnel(ctx context.Context, f repository.Funnel) (repository.Funnel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.funnels[f.ID]
	if !ok {
		return repository.Funnel{}, sql.ErrNoRows
	}
	f.ProjectID = existing.ProjectID
	f.CreatedAt = existing.CreatedAt
	for i := range f.Steps {
		f.Steps[i].ID = newMockID("step")
		f.Steps[i].FunnelID = f.ID
		f.Steps[i].StepOrder = i + 1
	}
	s.funnels[f.ID] = f
	return f, nil
}

func (s *Store) DeleteFunnel(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.funnels, id)
	return nil
}

func (s *Store) AnalyzeFunnel(ctx context.Context, f repository.Funnel, from, to time.Time, seg *repository.SegmentFilter) ([]repository.FunnelStepResult, error) {
	// Complex SQL — not mocked for logic tests.
	return []repository.FunnelStepResult{}, nil
}

func (s *Store) FunnelSegmentData(ctx context.Context, projectID string) (repository.FunnelSegments, error) {
	return repository.FunnelSegments{}, nil
}

// ── A/B Tests ─────────────────────────────────────────────────────────────────

func (s *Store) CreateABTest(ctx context.Context, t repository.ABTest) (repository.ABTest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.ID = newMockID("abtest")
	t.CreatedAt = time.Now()
	s.abtests[t.ID] = t
	return t, nil
}

func (s *Store) ListABTests(ctx context.Context, projectID string) ([]repository.ABTest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []repository.ABTest
	for _, t := range s.abtests {
		if t.ProjectID == projectID {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) ABTestByID(ctx context.Context, id string) (repository.ABTest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.abtests[id]
	if !ok {
		return repository.ABTest{}, sql.ErrNoRows
	}
	return t, nil
}

func (s *Store) AnalyzeABTest(ctx context.Context, t repository.ABTest, from, to time.Time) ([]repository.ABTestResult, error) {
	return []repository.ABTestResult{}, nil
}

// ── Sessions ──────────────────────────────────────────────────────────────────

func (s *Store) UpsertSession(ctx context.Context, sess repository.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.sessions[sess.ID]
	if ok {
		existing.LastSeenAt = sess.LastSeenAt
		existing.EventCount++
		existing.ExitURL = sess.ExitURL
		s.sessions[sess.ID] = existing
	} else {
		sess.EventCount = 1
		s.sessions[sess.ID] = sess
	}
	return nil
}

func (s *Store) SessionByID(ctx context.Context, id string) (repository.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return repository.Session{}, sql.ErrNoRows
	}
	return sess, nil
}

func (s *Store) ListSessions(ctx context.Context, projectID string, limit, offset int) ([]repository.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	var all []repository.Session
	for _, sess := range s.sessions {
		if sess.ProjectID == projectID {
			all = append(all, sess)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].LastSeenAt.After(all[j].LastSeenAt) })
	if offset >= len(all) {
		return []repository.Session{}, nil
	}
	all = all[offset:]
	if limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

func (s *Store) ActiveSessionCount(ctx context.Context, projectID string, withinMinutes int) (int64, error) {
	return 0, nil
}

// ── Events ────────────────────────────────────────────────────────────────────

func (s *Store) InsertEvent(ctx context.Context, e repository.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	return nil
}

func (s *Store) ListEvents(ctx context.Context, projectID string, limit, offset int) ([]repository.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	var all []repository.Event
	for _, e := range s.events {
		if e.ProjectID == projectID {
			all = append(all, e)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].OccurredAt.After(all[j].OccurredAt) })
	if offset >= len(all) {
		return []repository.Event{}, nil
	}
	all = all[offset:]
	if limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

func (s *Store) CountEvents(ctx context.Context, projectID string, from, to time.Time) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var n int64
	for _, e := range s.events {
		if e.ProjectID == projectID && !e.OccurredAt.Before(from) && !e.OccurredAt.After(to) {
			n++
		}
	}
	return n, nil
}

func (s *Store) TopPages(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.PageStat, error) {
	return []repository.PageStat{}, nil
}

func (s *Store) TopReferrers(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.ReferrerStat, error) {
	return []repository.ReferrerStat{}, nil
}

func (s *Store) DailyEventCounts(ctx context.Context, projectID string, from, to time.Time) ([]repository.TimeSeriesPoint, error) {
	return []repository.TimeSeriesPoint{}, nil
}

func (s *Store) DailyUniqueSessions(ctx context.Context, projectID string, from, to time.Time) ([]repository.TimeSeriesPoint, error) {
	return []repository.TimeSeriesPoint{}, nil
}

func (s *Store) TopBrowsers(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.BrowserStat, error) {
	return []repository.BrowserStat{}, nil
}

func (s *Store) TopDeviceTypes(ctx context.Context, projectID string, from, to time.Time) ([]repository.DeviceStat, error) {
	return []repository.DeviceStat{}, nil
}

func (s *Store) TopEventNames(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.EventNameStat, error) {
	return []repository.EventNameStat{}, nil
}

func (s *Store) TopUTMSources(ctx context.Context, projectID string, from, to time.Time, limit int) ([]repository.UTMStat, error) {
	return []repository.UTMStat{}, nil
}

func (s *Store) BounceRate(ctx context.Context, projectID string, from, to time.Time) (float64, error) {
	return 0, nil
}

func (s *Store) AvgEventsPerSession(ctx context.Context, projectID string, from, to time.Time) (float64, error) {
	return 0, nil
}

func (s *Store) UniqueSessionCount(ctx context.Context, projectID string, from, to time.Time) (int64, error) {
	return 0, nil
}

func (s *Store) GetEventByIngestID(ctx context.Context, ingestID string) (*repository.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.events {
		if e.IngestID == ingestID {
			ec := e
			return &ec, nil
		}
	}
	return nil, nil
}

func (s *Store) DistinctEventNames(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (s *Store) PurgeOldEvents(ctx context.Context, cutoff time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept []repository.Event
	var deleted int64
	for _, e := range s.events {
		if e.OccurredAt.Before(cutoff) {
			deleted++
		} else {
			kept = append(kept, e)
		}
	}
	s.events = kept
	return deleted, nil
}

func (s *Store) Ping(_ context.Context) error { return nil }
