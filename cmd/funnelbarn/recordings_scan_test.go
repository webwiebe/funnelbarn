package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/wiebe-xyz/funnelbarn/internal/enrich"
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
	case "budget":
		return service.ChunkURLScan{Recordings: 3, Chunks: 11}, fmt.Errorf("scan: %w", context.DeadlineExceeded)
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

type memObjects struct {
	data   map[string][]byte
	putErr error
	getErr error
	puts   int
}

func (m *memObjects) Get(_ context.Context, key string) ([]byte, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	b, ok := m.data[key]
	if !ok {
		return nil, &types.NoSuchKey{}
	}
	return b, nil
}

func (m *memObjects) Put(_ context.Context, key string, b []byte) error {
	if m.putErr != nil {
		return m.putErr
	}
	if m.data == nil {
		m.data = map[string][]byte{}
	}
	m.data[key] = b
	m.puts++
	return nil
}

func stateOptions(objects *memObjects, st *scanState) scanOptions {
	return scanOptions{state: st, save: func(s *scanState) error { return saveScanState(context.Background(), objects, s) }}
}

// A run cut off by its time budget leaves the finished projects in the saved
// state; the next run prints them from the state, scans only the rest and
// totals all of them.
func TestScanRecordingURLParams_TimeBudgetKeepsFinishedProjectsAndResumes(t *testing.T) {
	objects := &memObjects{}
	projects := fakeProjects{{ID: "p1", Slug: "iambarn"}, {ID: "budget", Slug: "geobarn"}, {ID: "p3", Slug: "later"}}
	scanner := fakeScanner{"p1": {Recordings: 4, Chunks: 10, MatchingChunks: 3}, "p3": {Recordings: 1, Chunks: 2}}

	var first bytes.Buffer
	err := scanRecordingURLParams(context.Background(), &first, projects, scanner, stateOptions(objects, &scanState{}))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want the deadline error, got %v", err)
	}
	if !strings.Contains(first.String(), `"stopped":"time budget"`) {
		t.Errorf("the stop must name the time budget:\n%s", first.String())
	}
	saved, err := loadScanState(context.Background(), objects)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := saved.Projects["iambarn"]; !ok || len(saved.Projects) != 1 {
		t.Fatalf("only the finished project is saved, got %v", saved.Projects)
	}

	// Second run: geobarn now finishes (a scanner that no longer runs out).
	projects[1].ID = "p2" // the same project, now with a scanner that finishes
	scanner["p2"] = service.ChunkURLScan{Recordings: 3, Chunks: 11}
	delete(scanner, "p1") // iambarn must come from the state, not from a new scan
	var second bytes.Buffer
	if err := scanRecordingURLParams(context.Background(), &second, projects, scanner, stateOptions(objects, saved)); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(second.String()), "\n")
	if !strings.Contains(lines[0], `"project":"iambarn"`) || !strings.Contains(lines[0], `"from_previous_run":true`) || !strings.Contains(lines[0], `"chunks":10`) {
		t.Errorf("iambarn comes from the saved state: %s", lines[0])
	}
	total := lines[len(lines)-1]
	for _, want := range []string{`"project":"(total)"`, `"recordings":8`, `"chunks":23`, `"matching_chunks":3`} {
		if !strings.Contains(total, want) {
			t.Errorf("total lacks %s: %s", want, total)
		}
	}
}

// A failed save does not stop the scan, and a spot check (no state) never saves.
func TestScanRecordingURLParams_SaveFailureDoesNotStopTheScan(t *testing.T) {
	objects := &memObjects{putErr: errors.New("r2 down")}
	projects := fakeProjects{{ID: "p1", Slug: "iambarn"}, {ID: "p3", Slug: "later"}}
	scanner := fakeScanner{"p1": {Recordings: 4, Chunks: 10}, "p3": {Recordings: 1, Chunks: 2}}
	var out bytes.Buffer
	if err := scanRecordingURLParams(context.Background(), &out, projects, scanner, stateOptions(objects, &scanState{})); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"warning":"state not saved"`) || !strings.Contains(out.String(), `"chunks":12`) {
		t.Errorf("want a warning and the full total:\n%s", out.String())
	}

	good := &memObjects{}
	if err := scanRecordingURLParams(context.Background(), &out, projects, scanner, scanOptions{}); err != nil || good.puts != 0 {
		t.Errorf("a run without state must not save: %v, %d puts", err, good.puts)
	}
}

func TestLoadScanState(t *testing.T) {
	s, err := loadScanState(context.Background(), &memObjects{})
	if err != nil || len(s.Projects) != 0 {
		t.Errorf("a missing object is an empty state: %v, %v", s, err)
	}
	if _, err := loadScanState(context.Background(), &memObjects{getErr: errors.New("timeout")}); err == nil {
		t.Error("a storage failure must not read as an empty state")
	}
	if _, err := loadScanState(context.Background(), &memObjects{data: map[string][]byte{scanStateKey: []byte("{")}}); err == nil {
		t.Error("a broken state object must be an error")
	}
}

func TestParseScanFlags_BudgetAndFresh(t *testing.T) {
	f, err := parseScanFlags([]string{"--max-duration", "55m", "--fresh"})
	if err != nil || f.maxDuration != 55*time.Minute || !f.fresh {
		t.Errorf("flags: %+v, %v", f, err)
	}
}

// The saved state is a record in a bucket that outlives the run, so it must
// hold no query value: shapes built from URLs that carry a secret keep the
// host, the masked path and the parameter name, and the object never holds the
// value, in any of its keys.
func TestScanState_HoldsNoQueryValues(t *testing.T) {
	const secret = "s3cr3t-token-value-0042"
	var shapes []service.URLShapeCount
	for _, raw := range []string{
		"https://id.example.com/verify/0b1c2d3e-aaaa-bbbb-cccc-1234567890ab?token=" + secret + "&tab=1",
		"https://id.example.com/callback?code=" + secret + "&state=" + secret + "#invite=" + secret,
	} {
		for _, sh := range enrich.SensitiveURLShapes(raw) {
			shapes = append(shapes, service.URLShapeCount{URLShape: sh, Chunks: 1, Recordings: 1})
		}
	}
	if len(shapes) == 0 {
		t.Fatal("the fixture URLs must yield shapes")
	}

	objects := &memObjects{}
	scanner := fakeScanner{"p1": {Recordings: 1, Chunks: 2, MatchingChunks: 2, MatchingRecordings: 1, Shapes: shapes}}
	opts := stateOptions(objects, &scanState{})
	var out bytes.Buffer
	if err := scanRecordingURLParams(context.Background(), &out, fakeProjects{{ID: "p1", Slug: "iambarn"}}, scanner, opts); err != nil {
		t.Fatal(err)
	}
	saved := string(objects.data[scanStateKey])
	if saved == "" {
		t.Fatal("the state was not saved")
	}
	if strings.Contains(saved, secret) || strings.Contains(out.String(), secret) {
		t.Errorf("a query value reached the state or the output:\n%s", saved)
	}
	for _, want := range []string{`"host":"id.example.com"`, `"param":"token"`} {
		if !strings.Contains(saved, want) {
			t.Errorf("the state lacks the shape %s:\n%s", want, saved)
		}
	}
}
