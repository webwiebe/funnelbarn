package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

type fakeProjects []repository.Project

func (f fakeProjects) ListProjects(context.Context) ([]repository.Project, error) { return f, nil }

type fakeScanner map[string]service.ChunkURLScan

func (f fakeScanner) ScanChunkURLs(_ context.Context, projectID string) (service.ChunkURLScan, error) {
	if projectID == "broken" {
		return service.ChunkURLScan{}, errors.New("boom")
	}
	return f[projectID], nil
}

func TestScanRecordingURLParams(t *testing.T) {
	t1 := time.Date(2026, 9, 8, 20, 27, 0, 0, time.UTC)
	t2 := time.Date(2026, 10, 13, 16, 40, 0, 0, time.UTC)
	projects := fakeProjects{{ID: "p1", Slug: "iambarn"}, {ID: "p2", Slug: "empty"}, {ID: "p3", Slug: "geobarn"}}
	scanner := fakeScanner{
		"p1": {Recordings: 4, Chunks: 10, MatchingChunks: 3, MatchingRecordings: 2, FirstMatch: &t1, LastMatch: &t1},
		"p3": {Recordings: 1, Chunks: 2, Unreadable: 1, MatchingChunks: 1, MatchingRecordings: 1, FirstMatch: &t2, LastMatch: &t2},
	}
	var out bytes.Buffer
	if err := scanRecordingURLParams(context.Background(), &out, projects, scanner); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines (two projects with recordings and a total), got %d:\n%s", len(lines), out.String())
	}
	if !strings.Contains(lines[0], `"project":"iambarn"`) || !strings.Contains(lines[1], `"project":"geobarn"`) {
		t.Errorf("project lines:\n%s", out.String())
	}
	total := lines[2]
	for _, want := range []string{`"project":"(total)"`, `"recordings":5`, `"chunks":12`, `"unreadable":1`,
		`"matching_chunks":4`, `"matching_recordings":3`, `"first_match":"2026-09-08T20:27:00Z"`, `"last_match":"2026-10-13T16:40:00Z"`} {
		if !strings.Contains(total, want) {
			t.Errorf("total line lacks %s: %s", want, total)
		}
	}

	if err := scanRecordingURLParams(context.Background(), &out, fakeProjects{{ID: "broken", Slug: "x"}}, scanner); err == nil {
		t.Error("want an error when a project scan fails")
	}
}
