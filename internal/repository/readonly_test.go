package repository

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedSchemaVersionIsNewestMigration(t *testing.T) {
	v, err := EmbeddedSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v < 39 {
		t.Fatalf("EmbeddedSchemaVersion = %d, want at least 39", v)
	}
}

func TestOpenReadOnlyRefusesWrites(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err := w.CreateProject(ctx, "A", "a"); err != nil {
		t.Fatal(err)
	}

	r, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if _, err := r.ProjectBySlug(ctx, "a"); err != nil {
		t.Fatalf("read through the read-only store: %v", err)
	}
	_, err = r.CreateProject(ctx, "B", "b")
	if err == nil || !strings.Contains(err.Error(), "readonly") {
		t.Fatalf("write through the read-only store: err = %v, want a readonly error", err)
	}

	got, err := r.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want, err := EmbeddedSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("SchemaVersion = %d, want %d", got, want)
	}
}

func TestSchemaVersionOfOlderDatabase(t *testing.T) {
	db := openAtVersion(t, 30)
	var path string
	if err := db.QueryRow(`SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	r, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got, err := r.SchemaVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != 30 {
		t.Fatalf("SchemaVersion = %d, want 30", got)
	}
}

func TestOpenReadOnlyNeedsAFile(t *testing.T) {
	if _, err := OpenReadOnly(":memory:"); err == nil {
		t.Fatal("OpenReadOnly(:memory:) succeeded")
	}
}
