package repository

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/XSAM/otelsql"
	"github.com/pressly/goose/v3"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/sqlcgen"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store wraps a SQLite database connection.
type Store struct {
	db       *sql.DB
	q        *sqlcgen.Queries
	statsReg metric.Registration
}

// Open opens the SQLite database at path and runs goose migrations.
func Open(path string) (*Store, error) {
	if path == "" {
		path = ".data/funnelbarn.db"
	}

	// NOTE: the driver is modernc.org/sqlite, which does NOT understand the
	// mattn/go-sqlite3 DSN params (_journal_mode/_busy_timeout/_foreign_keys) —
	// it silently ignores them, leaving foreign keys OFF. modernc expects
	// PRAGMAs expressed as repeated `_pragma=` params. Foreign keys are required
	// for the schema's ON DELETE CASCADE constraints to actually fire.
	//
	// NOTE: otelsql.Open resolves otel.GetMeterProvider() exactly once, right
	// here, and binds to it permanently — it does not react to a later
	// otel.SetMeterProvider call. Open must therefore run AFTER
	// tracing.InitMetrics has installed the real MeterProvider, or the
	// db.sql.* histograms are silently bound to the no-op default forever.
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := otelsql.Open("sqlite", dsn,
		otelsql.WithAttributes(semconv.DBSystemSqlite),
		otelsql.WithSpanOptions(otelsql.SpanOptions{
			OmitConnPrepare:      true,
			OmitConnResetSession: true,
			OmitRows:             true,
			DisableErrSkip:       true,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Connection-pool gauges (open/in-use/idle, wait counts). Uses
	// otel.GetMeterProvider() at call time — the caller (main.go's run())
	// must have already called tracing.InitMetrics before repository.Open so
	// this binds to the real provider instead of the no-op default; see the
	// comment on otelsql.Open above.
	statsReg, err := otelsql.RegisterDBStatsMetrics(db, otelsql.WithMeterProvider(otel.GetMeterProvider()))
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("register db stats metrics: %w", err)
	}
	// closeAll tears down both the stats callback registration and the DB
	// connection; used on every error path below now that both exist.
	closeAll := func() {
		statsReg.Unregister()
		db.Close()
	}

	// SQLite should use a single writer connection.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// Fail fast if foreign keys did not actually get enabled — a driver/DSN
	// mismatch would silently disable every ON DELETE CASCADE in the schema.
	var fkEnabled int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fkEnabled); err != nil {
		closeAll()
		return nil, fmt.Errorf("check foreign_keys pragma: %w", err)
	}
	if fkEnabled != 1 {
		closeAll()
		return nil, fmt.Errorf("foreign keys are not enabled (PRAGMA foreign_keys=%d); schema cascade constraints would not fire", fkEnabled)
	}

	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		closeAll()
		return nil, fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		closeAll()
		return nil, fmt.Errorf("goose up: %w", err)
	}

	// Backfill columns added to the schema after the initial migration was applied.
	// Safe to run on any DB regardless of how old it is.
	if err := ensureColumns(db); err != nil {
		closeAll()
		return nil, fmt.Errorf("ensure columns: %w", err)
	}

	return &Store{db: db, q: sqlcgen.New(db), statsReg: statsReg}, nil
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
	if s.statsReg != nil {
		s.statsReg.Unregister()
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
