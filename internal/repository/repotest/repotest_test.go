package repotest_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/repotest"
)

// A copy carries every migration, the same as a freshly migrated database.
func TestOpenIsFullyMigrated(t *testing.T) {
	want, err := repository.EmbeddedSchemaVersion()
	require.NoError(t, err)
	got, err := repotest.Open(t).SchemaVersion(context.Background())
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// Each test gets its own copy: a write in one store is absent from the next.
func TestCopiesAreIndependent(t *testing.T) {
	ctx := context.Background()
	first := repotest.Open(t)
	_, err := first.CreateProject(ctx, "One", "one")
	require.NoError(t, err)

	second := repotest.Open(t)
	projects, err := second.ListProjects(ctx)
	require.NoError(t, err)
	require.Empty(t, projects)
}
