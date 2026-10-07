package repository

import (
	"context"
	"database/sql"

	"github.com/wiebe-xyz/funnelbarn/internal/repository/sqlcgen"
)

// ReadStore is the query side of the repository. It holds only the read pool,
// so a method with a *ReadStore receiver has no write handle to misuse.
// Store embeds it and adds the write pool and the command methods.
type ReadStore struct {
	rdb *sql.DB
	rq  *sqlcgen.Queries
}

// ReadDB returns the read-only pool. For in-memory databases it is the write
// pool.
func (s *ReadStore) ReadDB() *sql.DB {
	return s.rdb
}

// Ping verifies the read pool connection is alive.
func (s *ReadStore) Ping(ctx context.Context) error {
	return s.rdb.PingContext(ctx)
}
