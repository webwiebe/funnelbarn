package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository/sqlcgen"
)

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
func (s *Store) EnsureProject(ctx context.Context, slug string) (Project, error) {
	p, err := s.ProjectBySlug(ctx, slug)
	if err == nil {
		return p, nil
	}
	if err != sql.ErrNoRows {
		return Project{}, err
	}
	return s.CreateProject(ctx, slug, slug)
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

// DeleteProject removes a project and all related data (cascades via FK constraints).
func (s *Store) DeleteProject(ctx context.Context, id string) error {
	return s.q.DeleteProject(ctx, id)
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
