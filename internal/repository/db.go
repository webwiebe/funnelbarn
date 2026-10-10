package repository

import (
	"database/sql"
	"embed"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/XSAM/otelsql"
	"github.com/pressly/goose/v3"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/sqlcgen"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store wraps a SQLite database connection.
//
// It embeds the ReadStore, which holds the read-only pool, and adds db, the
// single-connection write pool, for the command methods. For in-memory
// databases the read pool aliases db.
type Store struct {
	*ReadStore
	db           *sql.DB
	q            *sqlcgen.Queries
	statsReg     metric.Registration
	readStatsReg metric.Registration
}

// Reader returns the query side of the store. It holds no write handle.
func (s *Store) Reader() *ReadStore {
	return s.ReadStore
}

// poolAttr labels the otelsql metrics and spans of one pool.
func poolAttr(pool string) attribute.KeyValue {
	return attribute.String("db.pool", pool)
}

// readOnlyDSN builds the DSN of the read pool for the database at absPath.
func readOnlyDSN(absPath string) string {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(absPath)}
	return u.String() + "?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
}

// isFileDB reports whether path names an on-disk database that a second pool
// can open.
func isFileDB(path string) bool {
	if path == ":memory:" || strings.HasPrefix(path, "file:") || strings.Contains(path, "mode=memory") {
		return false
	}
	return true
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
		otelsql.WithAttributes(semconv.DBSystemSqlite, poolAttr("write")),
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
	statsReg, err := otelsql.RegisterDBStatsMetrics(db,
		otelsql.WithMeterProvider(otel.GetMeterProvider()),
		otelsql.WithAttributes(semconv.DBSystemSqlite, poolAttr("write")))
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

	st := &Store{
		ReadStore: &ReadStore{rdb: db, rq: sqlcgen.New(db)},
		db:        db,
		q:         sqlcgen.New(db),
		statsReg:  statsReg,
	}

	// The read pool opens after migrations so the schema exists and the WAL
	// shared-memory file has been created by the write pool.
	if isFileDB(path) {
		if err := st.openReadPool(path); err != nil {
			closeAll()
			return nil, err
		}
	}
	return st, nil
}

// openReadPool opens the read-only pool on the same file as the write pool.
func (s *Store) openReadPool(path string) error {
	rdb, reg, err := openReadOnlyPool(path)
	if err != nil {
		return err
	}
	s.rdb = rdb
	s.rq = sqlcgen.New(rdb)
	s.readStatsReg = reg
	return nil
}

// OpenReadOnly opens the database at path for a reader process: one read-only
// pool, no migrations and no backfill. The returned Store uses that pool on its
// write side too, so every command method fails with SQLITE_READONLY instead
// of writing. The writer process migrates the file; a reader compares
// SchemaVersion with EmbeddedSchemaVersion before it serves traffic.
func OpenReadOnly(path string) (*Store, error) {
	if !isFileDB(path) {
		return nil, fmt.Errorf("read-only open needs a database file, got %q", path)
	}
	rdb, reg, err := openReadOnlyPool(path)
	if err != nil {
		return nil, err
	}
	return &Store{
		ReadStore:    &ReadStore{rdb: rdb, rq: sqlcgen.New(rdb)},
		db:           rdb,
		q:            sqlcgen.New(rdb),
		readStatsReg: reg,
	}, nil
}

// openReadOnlyPool opens a read-only pool on the database file at path and
// checks that it refuses writes.
func openReadOnlyPool(path string) (*sql.DB, metric.Registration, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve db path: %w", err)
	}
	rdb, err := otelsql.Open("sqlite", readOnlyDSN(abs),
		otelsql.WithAttributes(semconv.DBSystemSqlite, poolAttr("read")),
		otelsql.WithSpanOptions(otelsql.SpanOptions{
			OmitConnPrepare:      true,
			OmitConnResetSession: true,
			OmitRows:             true,
			DisableErrSkip:       true,
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite read pool: %w", err)
	}
	rdb.SetMaxOpenConns(4)
	rdb.SetMaxIdleConns(4)
	rdb.SetConnMaxLifetime(0)

	reg, err := otelsql.RegisterDBStatsMetrics(rdb,
		otelsql.WithMeterProvider(otel.GetMeterProvider()),
		otelsql.WithAttributes(semconv.DBSystemSqlite, poolAttr("read")))
	if err != nil {
		_ = rdb.Close()
		return nil, nil, fmt.Errorf("register read pool db stats metrics: %w", err)
	}

	var qo int
	if err := rdb.QueryRow("PRAGMA query_only").Scan(&qo); err != nil {
		_ = reg.Unregister()
		_ = rdb.Close()
		return nil, nil, fmt.Errorf("check query_only pragma: %w", err)
	}
	if qo != 1 {
		_ = reg.Unregister()
		_ = rdb.Close()
		return nil, nil, fmt.Errorf("read pool is writable (PRAGMA query_only=%d)", qo)
	}
	return rdb, reg, nil
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

// Close closes both pools. When the read pool aliases the write pool it is
// closed once.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	var rerr error
	if s.rdb != nil && s.rdb != s.db {
		if s.readStatsReg != nil {
			_ = s.readStatsReg.Unregister()
		}
		rerr = s.rdb.Close()
	}
	if s.rdb == s.db && s.readStatsReg != nil {
		// OpenReadOnly: the one pool is the read pool.
		_ = s.readStatsReg.Unregister()
	}
	if s.statsReg != nil {
		s.statsReg.Unregister()
	}
	if err := s.db.Close(); err != nil {
		return err
	}
	return rerr
}

// DB returns the write pool for use by other packages.
func (s *Store) DB() *sql.DB {
	return s.db
}
