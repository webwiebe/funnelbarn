package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"
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
	CreatedAt time.Time `json:"created_at"`
}

// CreateProject inserts a new project.
func (s *Store) CreateProject(ctx context.Context, name, slug string) (Project, error) {
	id := newUUID()
	const q = `INSERT INTO projects (id, name, slug) VALUES (?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q, id, name, slug); err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return s.ProjectByID(ctx, id)
}

// ProjectByID fetches a project by its ID.
func (s *Store) ProjectByID(ctx context.Context, id string) (Project, error) {
	const q = `SELECT id, name, slug, status, created_at FROM projects WHERE id = ?`
	row := s.db.QueryRowContext(ctx, q, id)
	return scanProject(row)
}

// ProjectBySlug fetches a project by its slug.
func (s *Store) ProjectBySlug(ctx context.Context, slug string) (Project, error) {
	const q = `SELECT id, name, slug, status, created_at FROM projects WHERE slug = ?`
	row := s.db.QueryRowContext(ctx, q, slug)
	return scanProject(row)
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
	const q = `INSERT INTO projects (id, name, slug, status) VALUES (?, ?, ?, 'pending')`
	if _, err := s.db.ExecContext(ctx, q, id, name, slug); err != nil {
		return Project{}, fmt.Errorf("create pending project: %w", err)
	}
	return s.ProjectByID(ctx, id)
}

// ApproveProject sets status='active' for a project and returns the updated project.
func (s *Store) ApproveProject(ctx context.Context, id string) (Project, error) {
	const q = `UPDATE projects SET status = 'active' WHERE id = ?`
	if _, err := s.db.ExecContext(ctx, q, id); err != nil {
		return Project{}, fmt.Errorf("approve project: %w", err)
	}
	return s.ProjectByID(ctx, id)
}

// ListProjects returns all projects.
func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	const q = `SELECT id, name, slug, status, created_at FROM projects ORDER BY name`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.Status, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// DeleteProject removes a project and all related data (cascades via FK constraints).
func (s *Store) DeleteProject(ctx context.Context, id string) error {
	const q = `DELETE FROM projects WHERE id = ?`
	_, err := s.db.ExecContext(ctx, q, id)
	return err
}

// UpdateProject updates a project's name.
func (s *Store) UpdateProject(ctx context.Context, id, name string) (Project, error) {
	const q = `UPDATE projects SET name = ? WHERE id = ?`
	if _, err := s.db.ExecContext(ctx, q, name, id); err != nil {
		return Project{}, fmt.Errorf("update project: %w", err)
	}
	return s.ProjectByID(ctx, id)
}

// HasProjects returns true if at least one project exists in the database.
func (s *Store) HasProjects(ctx context.Context) (bool, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects`).Scan(&n)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	return n > 0, nil
}

func scanProject(row *sql.Row) (Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.Name, &p.Slug, &p.Status, &p.CreatedAt)
	if err != nil {
		return Project{}, err
	}
	return p, nil
}
