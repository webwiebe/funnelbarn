package repository

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository/sqlcgen"
)

// projectSlugPattern bounds slugs that may be auto-created on the ingest path.
// It mirrors the CLI's slugPattern (lowercase alphanumerics + single hyphens),
// so an attacker cannot use a forged x-funnelbarn-project header to spawn
// projects with arbitrary/garbage names.
var projectSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// APIKeyScopeFull allows full API access.
const APIKeyScopeFull = "full"

// APIKeyScopeIngest allows only event ingest.
const APIKeyScopeIngest = "ingest"

// Project represents a tracked website or application.
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Status    string    `json:"status"`
	Domain    string    `json:"domain"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateProject inserts a new project.
func (s *Store) CreateProject(ctx context.Context, name, slug string) (Project, error) {
	id := newUUID()
	if err := s.q.InsertProject(ctx, sqlcgen.InsertProjectParams{ID: id, Name: name, Slug: slug}); err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return s.ProjectByID(ctx, id)
}

// ProjectByID fetches a project by its ID.
func (s *Store) ProjectByID(ctx context.Context, id string) (Project, error) {
	p, err := s.q.GetProjectByID(ctx, id)
	if err != nil {
		return Project{}, err
	}
	return projectFromGen(p), nil
}

// ProjectBySlug fetches a project by its slug.
func (s *Store) ProjectBySlug(ctx context.Context, slug string) (Project, error) {
	p, err := s.q.GetProjectBySlug(ctx, slug)
	if err != nil {
		return Project{}, err
	}
	return projectFromGen(p), nil
}

// EnsureProject fetches a project by slug, creating it if absent.
//
// As a defence against misconfigured trackers that send a project's *ID* in
// the place of its slug (we have seen this happen multiple times — e.g. a
// site copies a UUID out of the dashboard URL into its x-funnelbarn-project
// header), if the supplied slug parses as a UUID and a project with that
// exact ID already exists, we route the event to that existing project
// instead of auto-creating a duplicate named after the UUID.
func (s *Store) EnsureProject(ctx context.Context, slug string) (Project, error) {
	p, err := s.ProjectBySlug(ctx, slug)
	if err == nil {
		return p, nil
	}
	if err != sql.ErrNoRows {
		return Project{}, err
	}

	if looksLikeUUID(slug) {
		if byID, idErr := s.ProjectByID(ctx, slug); idErr == nil {
			return byID, nil
		} else if idErr != sql.ErrNoRows {
			return Project{}, idErr
		}
		// UUID-shaped slug with no matching project either way — refuse to
		// auto-create. A UUID is never a meaningful slug; creating one
		// produces the "project shows up as a UUID in the picker" UX bug.
		return Project{}, fmt.Errorf("slug %q looks like a UUID but no project with that ID exists; refusing to auto-create", slug)
	}

	// Only auto-create for well-formed slugs. This blocks path-traversal /
	// garbage values (e.g. from a forged x-funnelbarn-project header on an
	// instance-wide key) from spawning junk projects.
	if !projectSlugPattern.MatchString(slug) {
		return Project{}, fmt.Errorf("refusing to auto-create project for invalid slug %q", slug)
	}

	return s.CreateProject(ctx, slug, slug)
}

// looksLikeUUID matches the canonical 8-4-4-4-12 hex layout, case-insensitive.
// We don't validate the variant/version bits because we only need to recognise
// "this is almost certainly a UUID someone pasted in", not strict RFC4122.
func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
	}
	return true
}

// EnsureProjectPending fetches a project by slug or creates it with status='pending'.
// If the project already exists (any status) it is returned as-is.
func (s *Store) EnsureProjectPending(ctx context.Context, name, slug string) (Project, error) {
	p, err := s.ProjectBySlug(ctx, slug)
	if err == nil {
		return p, nil
	}
	if err != sql.ErrNoRows {
		return Project{}, err
	}
	id := newUUID()
	if err := s.q.InsertProjectPending(ctx, sqlcgen.InsertProjectPendingParams{ID: id, Name: name, Slug: slug}); err != nil {
		return Project{}, fmt.Errorf("create pending project: %w", err)
	}
	return s.ProjectByID(ctx, id)
}

// ApproveProject sets status='active' for a project and returns the updated project.
func (s *Store) ApproveProject(ctx context.Context, id string) (Project, error) {
	if err := s.q.ApproveProject(ctx, id); err != nil {
		return Project{}, fmt.Errorf("approve project: %w", err)
	}
	return s.ProjectByID(ctx, id)
}

// ListProjects returns all projects.
func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.q.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	projects := make([]Project, 0, len(rows))
	for _, p := range rows {
		projects = append(projects, projectFromGen(p))
	}
	return projects, nil
}

// DeleteProject removes a project and all of its data in a single transaction.
// Child tables that declare ON DELETE CASCADE (events, sessions, recordings,
// feature_flags, flag_evaluations, dashboard_widgets, segments, canonical
// mappings, project_recording_settings, project_health, …) are removed
// automatically now that foreign keys are enforced (see Open). recording_traces
// has no foreign key, so it is deleted explicitly to avoid orphans. R2 chunk
// blobs are purged separately by the recording service before this call — SQLite
// deletion cannot reach object storage.
func (s *Store) DeleteProject(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete project: begin tx: %w", err)
	}
	defer tx.Rollback()

	// recording_traces has no FK to projects, so cascade won't reach it.
	if _, err := tx.ExecContext(ctx, `DELETE FROM recording_traces WHERE project_id = ?`, id); err != nil {
		return fmt.Errorf("delete project: recording_traces: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return tx.Commit()
}

// UpdateProject updates a project's name and domain.
func (s *Store) UpdateProject(ctx context.Context, id, name, domain string) (Project, error) {
	domainVal := sql.NullString{String: domain, Valid: domain != ""}
	if err := s.q.UpdateProject(ctx, sqlcgen.UpdateProjectParams{Name: name, Domain: domainVal, ID: id}); err != nil {
		return Project{}, fmt.Errorf("update project: %w", err)
	}
	return s.ProjectByID(ctx, id)
}

// HasProjects returns true if at least one project exists in the database.
func (s *Store) HasProjects(ctx context.Context) (bool, error) {
	n, err := s.q.CountProjects(ctx)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func projectFromGen(p sqlcgen.Project) Project {
	return Project{
		ID:        p.ID,
		Name:      p.Name,
		Slug:      p.Slug,
		Status:    p.Status,
		Domain:    p.Domain.String,
		CreatedAt: p.CreatedAt,
	}
}
