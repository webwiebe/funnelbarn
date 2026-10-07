package repository_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestReadPool_RejectsWritesAndSeesCommits(t *testing.T) {
	ctx := context.Background()
	s, err := repository.Open(filepath.Join(t.TempDir(), "rw.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	require.NotSame(t, s.DB(), s.ReadDB())
	require.NoError(t, s.Ping(ctx))

	_, err = s.ReadDB().ExecContext(ctx, `INSERT INTO projects (id, name, slug) VALUES ('x','x','x')`)
	require.Error(t, err, "read pool must reject writes")

	p, err := s.CreateProject(ctx, "Pool Project", "pool-project")
	require.NoError(t, err)

	var name string
	require.NoError(t, s.ReadDB().QueryRowContext(ctx, `SELECT name FROM projects WHERE id = ?`, p.ID).Scan(&name))
	require.Equal(t, "Pool Project", name)
}

func TestReadPool_AliasesWritePoolInMemory(t *testing.T) {
	s, err := repository.Open(":memory:")
	require.NoError(t, err)
	require.Same(t, s.DB(), s.ReadDB())
	require.NoError(t, s.Close())
}
