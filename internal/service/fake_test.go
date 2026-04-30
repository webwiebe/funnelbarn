package service_test

// fakeABTestRepo is an in-memory implementation of ports.ABTestRepo.
// It demonstrates the ports pattern: service tests that need no SQLite.

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

type fakeABTestRepo struct {
	tests map[string]repository.ABTest
}

func newFakeABTestRepo() *fakeABTestRepo {
	return &fakeABTestRepo{tests: make(map[string]repository.ABTest)}
}

func (f *fakeABTestRepo) CreateABTest(_ context.Context, t repository.ABTest) (repository.ABTest, error) {
	if t.ID == "" {
		t.ID = fmt.Sprintf("fake-%d", len(f.tests)+1)
	}
	t.CreatedAt = time.Now().UTC()
	f.tests[t.ID] = t
	return t, nil
}

func (f *fakeABTestRepo) ListABTests(_ context.Context, projectID string) ([]repository.ABTest, error) {
	var out []repository.ABTest
	for _, t := range f.tests {
		if t.ProjectID == projectID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeABTestRepo) ABTestByID(_ context.Context, id string) (repository.ABTest, error) {
	t, ok := f.tests[id]
	if !ok {
		return repository.ABTest{}, sql.ErrNoRows
	}
	return t, nil
}

func (f *fakeABTestRepo) AnalyzeABTest(_ context.Context, _ repository.ABTest, _, _ time.Time) ([]repository.ABTestResult, error) {
	return nil, nil // stub — analysis is repo-level
}

// ---------------------------------------------------------------------------
// Tests using the fake — no SQLite, no temp files.
// ---------------------------------------------------------------------------

func TestABTestService_CreateAndList_WithFake(t *testing.T) {
	repo := newFakeABTestRepo()
	svc := service.NewABTestService(repo)
	ctx := context.Background()

	test, err := svc.CreateABTest(ctx, repository.ABTest{
		ProjectID:       "proj-x",
		Name:            "Button Colour",
		Status:          "running",
		ConversionEvent: "checkout",
	})
	if err != nil {
		t.Fatalf("CreateABTest: %v", err)
	}
	if test.ID == "" {
		t.Error("expected non-empty ID")
	}

	tests, err := svc.ListABTests(ctx, "proj-x")
	if err != nil {
		t.Fatalf("ListABTests: %v", err)
	}
	if len(tests) != 1 {
		t.Fatalf("want 1 test, got %d", len(tests))
	}
	if tests[0].Name != "Button Colour" {
		t.Errorf("unexpected name: %q", tests[0].Name)
	}
}

func TestABTestService_GetABTest_NotFound_WithFake(t *testing.T) {
	svc := service.NewABTestService(newFakeABTestRepo())
	_, err := svc.GetABTest(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent test")
	}
}
