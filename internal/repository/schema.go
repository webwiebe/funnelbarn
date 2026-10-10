package repository

import (
	"context"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

// SchemaVersion returns the newest goose migration applied to the database, or
// 0 when none has been applied.
func (s *ReadStore) SchemaVersion(ctx context.Context) (int64, error) {
	var tables int
	if err := s.rdb.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'`).Scan(&tables); err != nil {
		return 0, fmt.Errorf("schema version: %w", err)
	}
	if tables == 0 {
		return 0, nil
	}
	var v int64
	if err := s.rdb.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied = 1`).Scan(&v); err != nil {
		return 0, fmt.Errorf("schema version: %w", err)
	}
	return v, nil
}

// EmbeddedSchemaVersion returns the newest migration built into this binary,
// which is the version Open migrates to.
func EmbeddedSchemaVersion() (int64, error) {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return 0, fmt.Errorf("read embedded migrations: %w", err)
	}
	var newest int64
	for _, e := range entries {
		prefix, _, ok := strings.Cut(e.Name(), "_")
		if !ok || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		v, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("migration %s: %w", e.Name(), err)
		}
		newest = max(newest, v)
	}
	return newest, nil
}
