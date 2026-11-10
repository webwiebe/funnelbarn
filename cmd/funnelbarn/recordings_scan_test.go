package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

type fakeProjects []repository.Project

func (f fakeProjects) ListProjects(context.Context) ([]repository.Project, error) { return f, nil }

type fakeScanner map[string]service.ChunkURLScan

func (f fakeScanner) ScanChunkURLs(_ context.Context, projectID string, opts service.ChunkScanOpts) (service.ChunkURLScan, error) {
	switch projectID {
	case "broken":
		return service.ChunkURLScan{}, errors.New("boom")
	case "slow":
		if opts.Progress != nil {
			opts.Progress(service.ChunkURLScan{Recordings: 1, Chunks: 5, MatchingChunks: 2})
		}
		return service.ChunkURLScan{Recordings: 2, Chunks: 9, MatchingChunks: 2}, fmt.Errorf("scan: %w", context.Canceled)
	case "capped":
		return service.ChunkURLScan{Recordings: 2, Chunks: 7, MatchingChunks: 1, PeakMemoryBytes: 70 << 20}, service.ErrMemoryCap
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
	if err := scanRecordingURLParams(context.Background(), &out, projects, scanner, scanOptions{}); err != nil {
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

	if err := scanRecordingURLParams(context.Background(), &out, fakeProjects{{ID: "broken", Slug: "x"}}, scanner, scanOptions{}); err == nil {
		t.Error("want an error when a project scan fails")
	}
}

func TestScanRecordingURLParams_OneProject(t *testing.T) {
	projects := fakeProjects{{ID: "p1", Slug: "iambarn"}, {ID: "p3", Slug: "geobarn"}}
	scanner := fakeScanner{"p1": {Recordings: 4, Chunks: 10}, "p3": {Recordings: 1, Chunks: 2}}
	var out bytes.Buffer
	if err := scanRecordingURLParams(context.Background(), &out, projects, scanner, scanOptions{project: "geobarn"}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"project":"geobarn"`) || !strings.Contains(lines[1], `"chunks":2`) {
		t.Errorf("want geobarn and a total of its counts only:\n%s", out.String())
	}
	if err := scanRecordingURLParams(context.Background(), &out, projects, scanner, scanOptions{project: "nope"}); err == nil {
		t.Error("want an error for an unknown slug")
	}
}

func TestScanRecordingURLParams_MemoryCapPrintsCountsAndStops(t *testing.T) {
	projects := fakeProjects{{ID: "p1", Slug: "iambarn"}, {ID: "capped", Slug: "geobarn"}, {ID: "p3", Slug: "later"}}
	scanner := fakeScanner{"p1": {Recordings: 4, Chunks: 10}, "p3": {Recordings: 1, Chunks: 2}}
	var out bytes.Buffer
	err := scanRecordingURLParams(context.Background(), &out, projects, scanner, scanOptions{})
	if !errors.Is(err, service.ErrMemoryCap) {
		t.Fatalf("want ErrMemoryCap, got %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("want iambarn, the stopped geobarn line and a total, got:\n%s", out.String())
	}
	if !strings.Contains(lines[1], `"project":"geobarn"`) || !strings.Contains(lines[1], `"stopped":"memory cap"`) || !strings.Contains(lines[1], `"chunks":7`) {
		t.Errorf("stopped line: %s", lines[1])
	}
	if !strings.Contains(lines[2], `"project":"(total)"`) || !strings.Contains(lines[2], `"chunks":17`) || !strings.Contains(lines[2], `"stopped":"memory cap"`) {
		t.Errorf("total line: %s", lines[2])
	}
	if strings.Contains(out.String(), "later") {
		t.Error("the scan must stop at the capped project")
	}
}

func TestParseScanFlags(t *testing.T) {
	f, err := parseScanFlags(nil)
	if err != nil || f.project != "" || f.maxMemoryMiB != 64 {
		t.Errorf("defaults: %+v, %v", f, err)
	}
	f, err = parseScanFlags([]string{"--project", "iambarn", "--max-memory-mib", "32"})
	if err != nil || f.project != "iambarn" || f.maxMemoryMiB != 32 || f.byURL {
		t.Errorf("flags: %+v, %v", f, err)
	}
	f, err = parseScanFlags([]string{"--by-url"})
	if err != nil || !f.byURL {
		t.Errorf("--by-url: %+v, %v", f, err)
	}
	for _, bad := range [][]string{{"--max-memory-mib", "0"}, {"extra"}, {"--nope"}} {
		if _, err := parseScanFlags(bad); err == nil {
			t.Errorf("%v: want an error", bad)
		}
	}
}

// A scan cut off by a signal (SIGTERM at the Job's deadline) prints what it
// has: progress lines while it ran, then the stopped project and the total.
func TestScanRecordingURLParams_InterruptedKeepsProgressAndCounts(t *testing.T) {
	projects := fakeProjects{{ID: "p1", Slug: "iambarn"}, {ID: "slow", Slug: "geobarn"}, {ID: "p3", Slug: "later"}}
	scanner := fakeScanner{"p1": {Recordings: 4, Chunks: 10}, "p3": {Recordings: 1, Chunks: 2}}
	var out bytes.Buffer
	err := scanRecordingURLParams(context.Background(), &out, projects, scanner, scanOptions{started: time.Now().Add(-90 * time.Second)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("want iambarn, a geobarn progress line, the stopped geobarn line and a total:\n%s", out.String())
	}
	if !strings.Contains(lines[1], `"project":"geobarn"`) || !strings.Contains(lines[1], `"progress":true`) || !strings.Contains(lines[1], `"chunks":5`) {
		t.Errorf("progress line: %s", lines[1])
	}
	if !strings.Contains(lines[2], `"stopped":"interrupted"`) || !strings.Contains(lines[2], `"chunks":9`) || !strings.Contains(lines[2], `"elapsed_s":90`) {
		t.Errorf("stopped line: %s", lines[2])
	}
	if !strings.Contains(lines[3], `"project":"(total)"`) || !strings.Contains(lines[3], `"chunks":19`) || !strings.Contains(lines[3], `"stopped":"interrupted"`) {
		t.Errorf("total line: %s", lines[3])
	}
	if strings.Contains(out.String(), "later") {
		t.Error("the scan must stop at the interrupted project")
	}
}
