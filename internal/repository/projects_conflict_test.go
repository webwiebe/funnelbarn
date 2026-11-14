package repository_test

import (
	"context"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

func TestCreateProject_DuplicateSlugIsConflict(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.CreateProject(ctx, "First", "dup-slug"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := store.CreateProject(ctx, "Second", "dup-slug")
	if !domain.IsConflict(err) {
		t.Fatalf("duplicate slug error = %v, want domain.ErrConflict", err)
	}
}
