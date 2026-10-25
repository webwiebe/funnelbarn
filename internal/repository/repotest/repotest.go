// Package repotest gives tests a migrated store without running the
// migrations for each test (issue #395).
//
// Under -race a fresh repository.Open spends about half a second on the goose
// migrations, and the repository, service, api and mcp tests open hundreds of
// stores. The first call in a test binary migrates one database and keeps it
// in memory; every call after that writes a copy to the test's TempDir.
// repository.Open still runs on the copy, so goose finds the schema current
// and applies nothing.
package repotest

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

var (
	once     sync.Once
	template []byte
	buildErr error
)

// Path writes a fresh copy of the migrated database to t.TempDir()/name and
// returns its path, for tests that pass a database path to the code under test.
func Path(t testing.TB, name string) string {
	t.Helper()
	once.Do(func() { template, buildErr = build() })
	if buildErr != nil {
		t.Fatalf("repotest: migrate template database: %v", buildErr)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, template, 0o600); err != nil {
		t.Fatalf("repotest: write %s: %v", path, err)
	}
	return path
}

// Open opens a store on a fresh copy of the migrated database and closes it
// when the test ends.
func Open(t testing.TB) *repository.Store {
	t.Helper()
	path := Path(t, "test.db")
	s, err := repository.Open(path)
	if err != nil {
		t.Fatalf("repotest: repository.Open(%s): %v", path, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func build() ([]byte, error) {
	dir, err := os.MkdirTemp("", "funnelbarn-repotest-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	s, err := repository.Open(filepath.Join(dir, "migrated.db"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()
	// VACUUM INTO writes one self-contained file with no WAL beside it, so
	// the bytes are the whole database.
	out := filepath.Join(dir, "template.db")
	if _, err := s.DB().Exec("VACUUM INTO ?", out); err != nil {
		return nil, err
	}
	return os.ReadFile(out)
}
