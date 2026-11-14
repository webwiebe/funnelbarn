// Package mock provides an in-memory implementation of the persistence ports for testing.
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
)

var mockIDCounter int64

func newMockID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, atomic.AddInt64(&mockIDCounter, 1))
}

// Store is a thread-safe in-memory implementation of the persistence ports.
type Store struct {
	mu       sync.RWMutex
	projects map[string]domain.Project
	apikeys  map[string]domain.APIKey // keyed by ID
	users    map[string]domain.User   // keyed by username
	funnels  map[string]domain.Funnel
	abtests  map[string]domain.ABTest
	sessions map[string]domain.Session
	events   []domain.Event
	widgets  map[string]domain.DashboardWidget
	flags    map[string]domain.FeatureFlag
	// evaluations records what RecordEvaluation was asked to write, so tests can
	// assert that config flags write nothing.
	evaluations []domain.FlagEvaluation
}

// New returns a fresh empty Store.
func New() *Store {
	return &Store{
		projects: make(map[string]domain.Project),
		apikeys:  make(map[string]domain.APIKey),
		users:    make(map[string]domain.User),
		funnels:  make(map[string]domain.Funnel),
		abtests:  make(map[string]domain.ABTest),
		sessions: make(map[string]domain.Session),
	}
}

// ── Projects ──────────────────────────────────────────────────────────────────

func (s *Store) CreateProject(ctx context.Context, name, slug string) (domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range s.projects {
		if p.Slug == slug {
			return domain.Project{}, fmt.Errorf("UNIQUE constraint failed: projects.slug: %w", domain.ErrConflict)
		}
	}

	p := domain.Project{
		ID:        newMockID("proj"),
		Name:      name,
		Slug:      slug,
		Status:    "active",
		CreatedAt: time.Now(),
	}
	s.projects[p.ID] = p
	return p, nil
}

func (s *Store) ProjectByID(ctx context.Context, id string) (domain.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.projects[id]
	if !ok {
		return domain.Project{}, sql.ErrNoRows
	}
	return p, nil
}

func (s *Store) ProjectBySlug(ctx context.Context, slug string) (domain.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.projects {
		if p.Slug == slug {
			return p, nil
		}
	}
	return domain.Project{}, sql.ErrNoRows
}

func (s *Store) ListProjects(ctx context.Context) ([]domain.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Project, 0, len(s.projects))
	for _, p := range s.projects {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Store) UpdateProject(ctx context.Context, id, name, siteDomain string) (domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[id]
	if !ok {
		return domain.Project{}, sql.ErrNoRows
	}
	p.Name = name
	p.Domain = siteDomain
	s.projects[id] = p
	return p, nil
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.projects, id)
	return nil
}

func (s *Store) ApproveProject(ctx context.Context, id string) (domain.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[id]
	if !ok {
		return domain.Project{}, sql.ErrNoRows
	}
	p.Status = "active"
	s.projects[id] = p
	return p, nil
}

func (s *Store) EnsureProject(ctx context.Context, slug string) (domain.Project, error) {
	p, err := s.ProjectBySlug(ctx, slug)
	if err == nil {
		return p, nil
	}
	if err != sql.ErrNoRows {
		return domain.Project{}, err
	}
	return s.CreateProject(ctx, slug, slug)
}

func (s *Store) EnsureProjectPending(ctx context.Context, name, slug string) (domain.Project, error) {
	p, err := s.ProjectBySlug(ctx, slug)
	if err == nil {
		return p, nil
	}
	if err != sql.ErrNoRows {
		return domain.Project{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	proj := domain.Project{
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
	s.apikeys[id] = domain.APIKey{
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
		u = domain.User{
			ID:        newMockID("user"),
			Username:  username,
			CreatedAt: time.Now(),
		}
	}
	u.PasswordHash = passwordHash
	s.users[username] = u
	return nil
}

func (s *Store) UserByUsername(ctx context.Context, username string) (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[username]
	if !ok {
		return domain.User{}, sql.ErrNoRows
	}
	return u, nil
}

// ── API Keys ──────────────────────────────────────────────────────────────────

func (s *Store) CreateAPIKey(ctx context.Context, name, projectID, keySHA256, scope string) (domain.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := newMockID("apikey")
	k := domain.APIKey{
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

func (s *Store) ListAPIKeys(ctx context.Context, projectID string) ([]domain.APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.APIKey
	for _, k := range s.apikeys {
		if k.ProjectID == projectID {
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) ListAllAPIKeys(ctx context.Context) ([]domain.APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.APIKey, 0, len(s.apikeys))
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

func (s *Store) CreateFunnel(ctx context.Context, f domain.Funnel) (domain.Funnel, error) {
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

func (s *Store) FunnelByID(ctx context.Context, id string) (domain.Funnel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.funnels[id]
	if !ok {
		return domain.Funnel{}, sql.ErrNoRows
	}
	return f, nil
}

func (s *Store) ListFunnels(ctx context.Context, projectID string) ([]domain.Funnel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.Funnel
	for _, f := range s.funnels {
		if f.ProjectID == projectID {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) UpdateFunnel(ctx context.Context, f domain.Funnel) (domain.Funnel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.funnels[f.ID]
	if !ok {
		return domain.Funnel{}, sql.ErrNoRows
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

func (s *Store) AnalyzeFunnel(ctx context.Context, f domain.Funnel, from, to time.Time, seg *domain.SegmentFilter, _ ...domain.SegmentRule) ([]domain.FunnelStepResult, error) {
	// Complex SQL — not mocked for logic tests.
	return []domain.FunnelStepResult{}, nil
}

func (s *Store) AnalyzeFunnelBreakdown(_ context.Context, _ domain.Funnel, _, _ time.Time, _ *domain.SegmentFilter, _ string, _ ...domain.SegmentRule) ([]domain.FunnelBreakdownGroup, error) {
	return []domain.FunnelBreakdownGroup{}, nil
}

func (s *Store) FunnelSegmentData(ctx context.Context, projectID string) (domain.FunnelSegments, error) {
	return domain.FunnelSegments{}, nil
}

func (s *Store) SessionsAtStep(_ context.Context, _ domain.Funnel, _ int, _, _ time.Time, _ int, _ ...domain.SegmentRule) ([]string, error) {
	return nil, nil
}

func (s *Store) SessionsForPage(_ context.Context, _, _ string, _, _ time.Time, _ int) ([]string, error) {
	return nil, nil
}

// ── A/B Tests ─────────────────────────────────────────────────────────────────

func (s *Store) CreateABTest(ctx context.Context, t domain.ABTest) (domain.ABTest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.ID = newMockID("abtest")
	t.CreatedAt = time.Now()
	s.abtests[t.ID] = t
	return t, nil
}

func (s *Store) ListABTests(ctx context.Context, projectID string) ([]domain.ABTest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.ABTest
	for _, t := range s.abtests {
		if t.ProjectID == projectID {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) ABTestByID(ctx context.Context, id string) (domain.ABTest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.abtests[id]
	if !ok {
		return domain.ABTest{}, sql.ErrNoRows
	}
	return t, nil
}

func (s *Store) AnalyzeABTest(ctx context.Context, t domain.ABTest, from, to time.Time) ([]domain.ABTestResult, error) {
	return []domain.ABTestResult{}, nil
}

// ── Sessions ──────────────────────────────────────────────────────────────────

// sessionKey mirrors the real table's primary key. Keyed on ID alone the fake
// would collapse two projects' sessions into one row and hide exactly the bug
// the composite key exists to prevent.
func sessionKey(projectID, id string) string { return projectID + "\x00" + id }

func (s *Store) UpsertSession(ctx context.Context, sess domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sessionKey(sess.ProjectID, sess.ID)
	existing, ok := s.sessions[key]
	if ok {
		existing.LastSeenAt = sess.LastSeenAt
		existing.EventCount++
		existing.ExitURL = sess.ExitURL
		s.sessions[key] = existing
	} else {
		sess.EventCount = 1
		s.sessions[key] = sess
	}
	return nil
}

func (s *Store) SessionByID(ctx context.Context, projectID, id string) (domain.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[sessionKey(projectID, id)]
	if !ok {
		return domain.Session{}, sql.ErrNoRows
	}
	return sess, nil
}

func (s *Store) ListSessions(ctx context.Context, projectID string, limit, offset int) ([]domain.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	var all []domain.Session
	for _, sess := range s.sessions {
		if sess.ProjectID == projectID {
			all = append(all, sess)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].LastSeenAt.After(all[j].LastSeenAt) })
	if offset >= len(all) {
		return []domain.Session{}, nil
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

func (s *Store) InsertEvent(ctx context.Context, e domain.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	return nil
}

// PersistEvent mirrors repository.Store.PersistEvent: an ingest_id that is
// already stored changes nothing, otherwise the event is stored and the
// session upserted. Signals are not tracked by the mock.
func (s *Store) PersistEvent(ctx context.Context, e domain.Event, sess domain.Session, _ *domain.SessionSignals) (bool, error) {
	if existing, _ := s.GetEventByIngestID(ctx, e.IngestID); existing != nil {
		return false, nil
	}
	if err := s.InsertEvent(ctx, e); err != nil {
		return false, err
	}
	return true, s.UpsertSession(ctx, sess)
}

func (s *Store) ListEvents(ctx context.Context, projectID string, limit, offset int) ([]domain.Event, error) {
	return s.ListEventsFiltered(ctx, projectID, domain.EventListFilter{}, limit, offset)
}

func (s *Store) ListEventsFiltered(ctx context.Context, projectID string, f domain.EventListFilter, limit, offset int) ([]domain.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	var all []domain.Event
	for _, e := range s.events {
		if e.ProjectID != projectID || !matchesEventFilter(e, f) {
			continue
		}
		all = append(all, e)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].OccurredAt.After(all[j].OccurredAt) })
	if offset >= len(all) {
		return []domain.Event{}, nil
	}
	all = all[offset:]
	if limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

func matchesEventFilter(e domain.Event, f domain.EventListFilter) bool {
	switch {
	case f.Name != "" && e.Name != f.Name:
		return false
	case f.Environment != "" && e.Environment != f.Environment:
		return false
	case !f.From.IsZero() && e.OccurredAt.Before(f.From):
		return false
	case !f.To.IsZero() && e.OccurredAt.After(f.To):
		return false
	}
	return true
}

func (s *Store) CountEvents(ctx context.Context, projectID string, from, to time.Time, env string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var n int64
	for _, e := range s.events {
		if e.ProjectID == projectID && !e.OccurredAt.Before(from) && !e.OccurredAt.After(to) && (env == "" || e.Environment == env) {
			n++
		}
	}
	return n, nil
}

func (s *Store) TopPages(_ context.Context, _ string, _, _ time.Time, _ int, _ string) ([]domain.PageStat, error) {
	return []domain.PageStat{}, nil
}

func (s *Store) TopReferrers(_ context.Context, _ string, _, _ time.Time, _ int, _ string) ([]domain.ReferrerStat, error) {
	return []domain.ReferrerStat{}, nil
}

func (s *Store) DailyEventCounts(_ context.Context, _ string, _, _ time.Time, _ string) ([]domain.TimeSeriesPoint, error) {
	return []domain.TimeSeriesPoint{}, nil
}

func (s *Store) HourlyEventCounts(_ context.Context, _ string, _, _ time.Time, _ string) ([]domain.TimeSeriesPoint, error) {
	return []domain.TimeSeriesPoint{}, nil
}

func (s *Store) DailyUniqueSessions(_ context.Context, _ string, _, _ time.Time, _ string) ([]domain.TimeSeriesPoint, error) {
	return []domain.TimeSeriesPoint{}, nil
}

func (s *Store) TopBrowsers(_ context.Context, _ string, _, _ time.Time, _ int, _ string) ([]domain.BrowserStat, error) {
	return []domain.BrowserStat{}, nil
}

func (s *Store) TopDeviceTypes(_ context.Context, _ string, _, _ time.Time, _ string) ([]domain.DeviceStat, error) {
	return []domain.DeviceStat{}, nil
}

func (s *Store) TopEventNamesFiltered(_ context.Context, _ string, _, _ time.Time, _ int, _ domain.EventCountFilter) ([]domain.EventNameStat, error) {
	return nil, nil
}

func (s *Store) EventBreakdown(_ context.Context, _, _, _ string, _, _ time.Time, _ int, _ domain.EventCountFilter) ([]domain.BreakdownRow, error) {
	return nil, nil
}

func (s *Store) TopEventNames(_ context.Context, _ string, _, _ time.Time, _ int, _ string) ([]domain.EventNameStat, error) {
	return []domain.EventNameStat{}, nil
}

func (s *Store) TopUTMSources(_ context.Context, _ string, _, _ time.Time, _ int, _ string) ([]domain.UTMStat, error) {
	return []domain.UTMStat{}, nil
}

func (s *Store) BounceRate(_ context.Context, _ string, _, _ time.Time, _ string) (float64, error) {
	return 0, nil
}

func (s *Store) AvgEventsPerSession(_ context.Context, _ string, _, _ time.Time, _ string) (float64, error) {
	return 0, nil
}

func (s *Store) UniqueSessionCount(_ context.Context, _ string, _, _ time.Time, _ string) (int64, error) {
	return 0, nil
}

func (s *Store) DistinctEnvironments(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (s *Store) GetEventByIngestID(ctx context.Context, ingestID string) (*domain.Event, error) {
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

func (s *Store) DistinctEventProperties(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}

func (s *Store) DistinctPropertyValues(_ context.Context, _, _, _ string, _ int) ([]string, error) {
	return nil, nil
}

func (s *Store) PopulatedMetadataColumns(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}

func (s *Store) PageFlows(_ context.Context, _ string, _ string, _ int, _, _ time.Time, _ string) (domain.PageFlowResult, error) {
	return domain.PageFlowResult{Nodes: []domain.FlowNode{}, Links: []domain.FlowLink{}}, nil
}

func (s *Store) PurgeOldEvents(ctx context.Context, cutoff time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept []domain.Event
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

// ── Feature Flags ────────────────────────────────────────────────────────────

func (s *Store) CreateFlag(_ context.Context, f domain.FeatureFlag) (domain.FeatureFlag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f.ID = newMockID("flag")
	f.CreatedAt = time.Now()
	if f.Origin == "" {
		f.Origin = "manual"
	}
	if f.Kind == "" {
		f.Kind = domain.FlagKindExperiment
	}
	if s.flags == nil {
		s.flags = make(map[string]domain.FeatureFlag)
	}
	s.flags[f.ID] = f
	return f, nil
}

func (s *Store) EnsureAutoFlag(ctx context.Context, f domain.FeatureFlag) (domain.FeatureFlag, error) {
	s.mu.Lock()
	for _, existing := range s.flags {
		if existing.ProjectID == f.ProjectID && existing.FlagKey == f.FlagKey {
			s.mu.Unlock()
			return existing, nil
		}
	}
	s.mu.Unlock()
	if f.Origin == "" {
		f.Origin = "auto"
	}
	return s.CreateFlag(ctx, f)
}

func (s *Store) CountAutoFlags(_ context.Context, projectID string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, f := range s.flags {
		if f.ProjectID == projectID && f.Origin == "auto" {
			n++
		}
	}
	return n, nil
}

func (s *Store) TouchFlagEvaluated(_ context.Context, flagID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := s.flags[flagID]; ok {
		now := time.Now()
		f.LastEvaluatedAt = &now
		s.flags[flagID] = f
	}
	return nil
}

func (s *Store) PurgeStaleAutoFlags(_ context.Context, cutoff time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var deleted int64
	for id, f := range s.flags {
		if f.Origin != "auto" || f.Status != "inactive" {
			continue
		}
		seen := f.CreatedAt
		if f.LastEvaluatedAt != nil {
			seen = *f.LastEvaluatedAt
		}
		if seen.Before(cutoff) {
			delete(s.flags, id)
			deleted++
		}
	}
	return deleted, nil
}

func (s *Store) FlagByID(_ context.Context, id string) (domain.FeatureFlag, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.flags[id]
	if !ok {
		return domain.FeatureFlag{}, sql.ErrNoRows
	}
	return f, nil
}

func (s *Store) FlagByKey(_ context.Context, projectID, flagKey string) (domain.FeatureFlag, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.flags {
		if f.ProjectID == projectID && f.FlagKey == flagKey {
			return f, nil
		}
	}
	return domain.FeatureFlag{}, sql.ErrNoRows
}

func (s *Store) ListFlags(_ context.Context, projectID string) ([]domain.FeatureFlag, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.FeatureFlag
	for _, f := range s.flags {
		if f.ProjectID == projectID {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) UpdateFlag(_ context.Context, f domain.FeatureFlag) (domain.FeatureFlag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.flags[f.ID]
	if !ok {
		return domain.FeatureFlag{}, sql.ErrNoRows
	}
	f.ProjectID = existing.ProjectID
	f.FlagKey = existing.FlagKey
	f.CreatedAt = existing.CreatedAt
	f.Origin = "manual" // a human edit claims the flag
	if f.Kind == "" {
		f.Kind = existing.Kind
	}
	s.flags[f.ID] = f
	return f, nil
}

func (s *Store) DeleteFlag(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.flags, id)
	return nil
}

// RecordedEvaluations returns the evaluation rows RecordEvaluation was asked to
// write, so tests can assert that a config flag writes none.
func (s *Store) RecordedEvaluations() []domain.FlagEvaluation {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.FlagEvaluation, len(s.evaluations))
	copy(out, s.evaluations)
	return out
}

func (s *Store) RecordEvaluation(_ context.Context, e domain.FlagEvaluation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evaluations = append(s.evaluations, e)
	return nil
}

func (s *Store) CountEvaluationsByVariant(_ context.Context, _ string, _, _ time.Time) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (s *Store) CountConversionsByVariant(_ context.Context, _, _, _ string, _, _ time.Time) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (s *Store) PurgeOldEvaluations(_ context.Context, cutoff time.Time) (int64, error) {
	return 0, nil
}

// ── Widgets ──────────────────────────────────────────────────────────────────

func (s *Store) CreateWidget(_ context.Context, w domain.DashboardWidget) (domain.DashboardWidget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.ID = newMockID("widget")
	w.CreatedAt = time.Now()
	if s.widgets == nil {
		s.widgets = make(map[string]domain.DashboardWidget)
	}
	s.widgets[w.ID] = w
	return w, nil
}

func (s *Store) WidgetByID(_ context.Context, id string) (domain.DashboardWidget, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.widgets[id]
	if !ok {
		return domain.DashboardWidget{}, sql.ErrNoRows
	}
	return w, nil
}

func (s *Store) ListWidgets(_ context.Context, projectID string) ([]domain.DashboardWidget, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.DashboardWidget
	for _, w := range s.widgets {
		if w.ProjectID == projectID {
			out = append(out, w)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Position != out[j].Position {
			return out[i].Position < out[j].Position
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) UpdateWidget(_ context.Context, w domain.DashboardWidget) (domain.DashboardWidget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.widgets[w.ID]
	if !ok {
		return domain.DashboardWidget{}, sql.ErrNoRows
	}
	w.ProjectID = existing.ProjectID
	w.CreatedAt = existing.CreatedAt
	s.widgets[w.ID] = w
	return w, nil
}

func (s *Store) DeleteWidget(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.widgets, id)
	return nil
}

func (s *Store) WidgetBreakdown(_ context.Context, _, _, _ string, _, _ int) ([]domain.PropertyBreakdown, error) {
	return []domain.PropertyBreakdown{}, nil
}

func (s *Store) Ping(_ context.Context) error { return nil }
