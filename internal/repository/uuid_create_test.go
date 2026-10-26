package repository

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var errNoEntropy = errors.New("no entropy")

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errNoEntropy }

// Spec 004 FR-004: when crypto/rand fails, every create call returns the error
// and writes nothing. Before, newUUID dropped the error and the row went in with
// an empty id.
func TestCreateCalls_ReturnUUIDError(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "uuid.db"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	ctx := context.Background()

	project, err := s.CreateProject(ctx, "Existing", "existing")
	require.NoError(t, err)

	uuidSource = failingReader{}
	t.Cleanup(func() { uuidSource = defaultUUIDSource })

	calls := map[string]func() error{
		"UpsertUser": func() error { return s.UpsertUser(ctx, "alice", "hash") },
		"CreateIAMBarnUser": func() error {
			_, err := s.CreateIAMBarnUser(ctx, "sub-1", "bob")
			return err
		},
		"EnsureSetupAPIKey": func() error { return s.EnsureSetupAPIKey(ctx, project.ID, "setup-hash") },
		"CreateAPIKey": func() error {
			_, err := s.CreateAPIKey(ctx, "ci", project.ID, "key-hash", "ingest")
			return err
		},
		"CreateProject": func() error {
			_, err := s.CreateProject(ctx, "New", "new")
			return err
		},
		"EnsureProjectPending": func() error {
			_, err := s.EnsureProjectPending(ctx, "Pending", "pending")
			return err
		},
	}
	for name, call := range calls {
		require.ErrorIs(t, call(), errNoEntropy, name)
	}

	count := func(table string) int {
		var n int
		require.NoError(t, s.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n))
		return n
	}
	require.Equal(t, 0, count("users"))
	require.Equal(t, 0, count("api_keys"))
	require.Equal(t, 1, count("projects"))
}
