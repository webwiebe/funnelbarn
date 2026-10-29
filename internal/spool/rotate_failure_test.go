package spool_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

var errInjected = errors.New("injected failure")

func failingRename(string, string) error { return errInjected }

// openFailingOnce fails the first reopen and opens normally after that.
func openFailingOnce() func(string) (*os.File, error) {
	failed := false
	return func(path string) (*os.File, error) {
		if !failed {
			failed = true
			return nil, errInjected
		}
		return spool.OpenActive(path)
	}
}

func activeIDs(t *testing.T, dir string) []string {
	t.Helper()
	records, err := spool.ReadRecords(spool.Path(dir))
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	ids := make([]string, len(records))
	for i, r := range records {
		ids[i] = r.IngestID
	}
	return ids
}

func archiveCount(t *testing.T, dir string) int {
	t.Helper()
	archives, err := filepath.Glob(filepath.Join(dir, "ingest-*.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	return len(archives)
}

// Rotate closes the active file before it renames it. When the rename failed,
// the handle stayed closed and every later Append returned "file already
// closed", so ingest was down until the process restarted.
func TestRotate_FailedRenameKeepsSpoolWritable(t *testing.T) {
	dir := newTestDir(t)
	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()
	if err := sp.Append(makeRecord("before", `{}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	spool.SetFileOps(sp, failingRename, spool.OpenActive)

	if err := sp.Rotate(); !errors.Is(err, errInjected) {
		t.Fatalf("Rotate = %v, want the rename error", err)
	}
	if err := sp.Append(makeRecord("after", `{}`)); err != nil {
		t.Fatalf("Append after a failed rename: %v", err)
	}
	if err := sp.RotateIfExceeds(1 << 30); err != nil {
		t.Fatalf("RotateIfExceeds after a failed rename: %v", err)
	}

	// Nothing was archived, so both records are in the active file in order.
	if got := activeIDs(t, dir); len(got) != 2 || got[0] != "before" || got[1] != "after" {
		t.Fatalf("active records = %v, want [before after]", got)
	}
	if n := archiveCount(t, dir); n != 0 {
		t.Fatalf("%d archives after a failed rename, want 0", n)
	}
}

// When the rename succeeded but opening the new active file failed, the spool
// had no open file at all. The next Append opens it.
func TestRotate_FailedReopenRecoversOnNextAppend(t *testing.T) {
	dir := newTestDir(t)
	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()
	if err := sp.Append(makeRecord("archived", `{}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	spool.SetFileOps(sp, os.Rename, openFailingOnce())

	if err := sp.Rotate(); !errors.Is(err, errInjected) {
		t.Fatalf("Rotate = %v, want the reopen error", err)
	}
	if err := sp.RotateIfExceeds(1 << 30); err != nil {
		t.Fatalf("RotateIfExceeds with no open file: %v", err)
	}
	if err := sp.Append(makeRecord("after", `{}`)); err != nil {
		t.Fatalf("Append after a failed reopen: %v", err)
	}
	if err := sp.AppendBatch([]spool.Record{makeRecord("batch", `{}`)}); err != nil {
		t.Fatalf("AppendBatch after a failed reopen: %v", err)
	}

	if got := activeIDs(t, dir); len(got) != 2 || got[0] != "after" || got[1] != "batch" {
		t.Fatalf("active records = %v, want [after batch]", got)
	}
	if n := archiveCount(t, dir); n != 1 {
		t.Fatalf("%d archives, want the 1 the rename made", n)
	}
}

// Rename and reopen both fail: the spool reopens the unrenamed file on the
// next Append and keeps every record.
func TestRotate_FailedRenameAndReopen(t *testing.T) {
	dir := newTestDir(t)
	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()
	if err := sp.Append(makeRecord("before", `{}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	spool.SetFileOps(sp, failingRename, openFailingOnce())

	if err := sp.Rotate(); !errors.Is(err, errInjected) {
		t.Fatalf("Rotate = %v, want the injected error", err)
	}
	if err := sp.Append(makeRecord("after", `{}`)); err != nil {
		t.Fatalf("Append after a failed rename and reopen: %v", err)
	}
	if got := activeIDs(t, dir); len(got) != 2 || got[0] != "before" || got[1] != "after" {
		t.Fatalf("active records = %v, want [before after]", got)
	}
}

// An Append that cannot reopen the file reports the error, and Close on a
// spool with no open file still succeeds.
func TestAppend_ReopenFailureIsReported(t *testing.T) {
	dir := newTestDir(t)
	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	alwaysFail := func(string) (*os.File, error) { return nil, errInjected }
	spool.SetFileOps(sp, os.Rename, alwaysFail)

	if err := sp.Rotate(); !errors.Is(err, errInjected) {
		t.Fatalf("Rotate = %v, want the reopen error", err)
	}
	if err := sp.Append(makeRecord("lost", `{}`)); !errors.Is(err, errInjected) {
		t.Fatalf("Append = %v, want the reopen error", err)
	}
	if err := sp.Close(); err != nil {
		t.Fatalf("Close with no open file: %v", err)
	}
	if err := sp.Append(makeRecord("late", `{}`)); err == nil {
		t.Fatal("Append after Close succeeded, want an error")
	}
}
