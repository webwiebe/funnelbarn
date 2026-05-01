package repository

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/sqlcgen"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store wraps a SQLite database connection.
type Store struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

// Open opens the SQLite database at path and runs goose migrations.
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

	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		db.Close()
		return nil, fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		db.Close()
		return nil, fmt.Errorf("goose up: %w", err)
	}

	// Backfill columns added to the schema after the initial migration was applied.
	// Safe to run on any DB regardless of how old it is.
	if err := ensureColumns(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensure columns: %w", err)
	}

	return &Store{db: db, q: sqlcgen.New(db)}, nil
}

// ensureColumns adds any columns that may be missing on databases older than
// the migration that introduced them. Uses IF NOT EXISTS semantics via PRAGMA.
func ensureColumns(db *sql.DB) error {
	type colCheck struct{ table, column, def string }
	checks := []colCheck{
		{"projects", "status", "TEXT NOT NULL DEFAULT 'active'"},
	}
	for _, c := range checks {
		rows, err := db.Query(`PRAGMA table_info(` + c.table + `)`)
		if err != nil {
			return err
		}
		found := false
		for rows.Next() {
			var cid int
			var name, typ string
			var notNull int
			var dflt sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
				rows.Close()
				return err
			}
			if name == c.column {
				found = true
				break
			}
		}
		rows.Close()
		if !found {
			if _, err := db.Exec(`ALTER TABLE ` + c.table + ` ADD COLUMN ` + c.column + ` ` + c.def); err != nil {
				return fmt.Errorf("add column %s.%s: %w", c.table, c.column, err)
			}
		}
	}
	return nil
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

// Ping verifies the database connection is alive.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}
