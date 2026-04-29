package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps a SQLite database connection.
type Store struct {
	db *sql.DB
}

// Open opens the SQLite database at path and runs migrations.
func Open(path string) (*Store, error) {
	if path == "" {
		path = ".data/funnelbarn.db"
	}

	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite should use a single writer connection.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if _, err := db.Exec(Schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("run schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB returns the underlying *sql.DB for use by other packages.
func (s *Store) DB() *sql.DB {
	return s.db
}

// ValidAPIKeySHA256 looks up an API key by its SHA256 hex digest.
// Returns (projectID, scope, true, nil) on match.
func (s *Store) ValidAPIKeySHA256(ctx context.Context, keySHA256 string) (projectID string, scope string, found bool, err error) {
	const q = `
		SELECT project_id, scope FROM api_keys
		WHERE key_hash = ? AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
		LIMIT 1`
	row := s.db.QueryRowContext(ctx, q, keySHA256)
	err = row.Scan(&projectID, &scope)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return projectID, scope, true, nil
}

// TouchAPIKey updates last_used_at for a key by its SHA256 hash.
func (s *Store) TouchAPIKey(ctx context.Context, keySHA256 string) error {
	const q = `UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE key_hash = ?`
	_, err := s.db.ExecContext(ctx, q, keySHA256)
	return err
}

// --------------------------------------------------------------------------
// Project management
// --------------------------------------------------------------------------

// Project represents a tracked website or application.
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
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
	const q = `SELECT id, name, slug, created_at FROM projects WHERE id = ?`
	row := s.db.QueryRowContext(ctx, q, id)
	return scanProject(row)
}

// ProjectBySlug fetches a project by its slug.
func (s *Store) ProjectBySlug(ctx context.Context, slug string) (Project, error) {
	const q = `SELECT id, name, slug, created_at FROM projects WHERE slug = ?`
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

// ListProjects returns all projects.
func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	const q = `SELECT id, name, slug, created_at FROM projects ORDER BY name`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

func scanProject(row *sql.Row) (Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.Name, &p.Slug, &p.CreatedAt)
	if err != nil {
		return Project{}, err
	}
	return p, nil
}

// --------------------------------------------------------------------------
// User management
// --------------------------------------------------------------------------

// User represents an admin user.
type User struct {
	ID           string
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

// UpsertUser inserts or updates a user by username.
func (s *Store) UpsertUser(ctx context.Context, username, passwordHash string) error {
	const q = `
		INSERT INTO users (id, username, password_hash)
		VALUES (?, ?, ?)
		ON CONFLICT(username) DO UPDATE SET password_hash = excluded.password_hash`
	_, err := s.db.ExecContext(ctx, q, newUUID(), username, passwordHash)
	return err
}

// UserByUsername fetches a user by username.
func (s *Store) UserByUsername(ctx context.Context, username string) (User, error) {
	const q = `SELECT id, username, password_hash, created_at FROM users WHERE username = ?`
	var u User
	err := s.db.QueryRowContext(ctx, q, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// --------------------------------------------------------------------------
// API key management
// --------------------------------------------------------------------------

// APIKey represents a stored API key.
type APIKey struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	KeyHash   string    `json:"-"`
	Scope     string    `json:"scope"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateAPIKey inserts a new API key.
func (s *Store) CreateAPIKey(ctx context.Context, name, projectID, keySHA256, scope string) (APIKey, error) {
	id := newUUID()
	const q = `INSERT INTO api_keys (id, project_id, name, key_hash, scope) VALUES (?, ?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q, id, projectID, name, keySHA256, scope); err != nil {
		return APIKey{}, fmt.Errorf("create api key: %w", err)
	}
	var k APIKey
	const sel = `SELECT id, project_id, name, key_hash, scope, created_at FROM api_keys WHERE id = ?`
	err := s.db.QueryRowContext(ctx, sel, id).Scan(&k.ID, &k.ProjectID, &k.Name, &k.KeyHash, &k.Scope, &k.CreatedAt)
	return k, err
}

// ListAPIKeys returns all API keys for a project.
func (s *Store) ListAPIKeys(ctx context.Context, projectID string) ([]APIKey, error) {
	const q = `SELECT id, project_id, name, key_hash, scope, created_at FROM api_keys WHERE project_id = ? ORDER BY created_at`
	rows, err := s.db.QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.Name, &k.KeyHash, &k.Scope, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// DeleteAPIKey removes an API key by its ID.
func (s *Store) DeleteAPIKey(ctx context.Context, id string) error {
	const q = `DELETE FROM api_keys WHERE id = ?`
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

// ListAllAPIKeys returns all API keys across all projects, ordered by creation time.
func (s *Store) ListAllAPIKeys(ctx context.Context) ([]APIKey, error) {
	const q = `SELECT id, project_id, name, key_hash, scope, created_at FROM api_keys ORDER BY created_at`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.Name, &k.KeyHash, &k.Scope, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}
