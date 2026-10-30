package spool_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

// A worker tick that is already running when shutdown closes the spool reaches
// RotateIfExceeds with a closed file. Before, that returned "stat ...: file
// already closed", which the worker logged at Error and BugBarn filed as FUN-58
// on every deploy. The order is reproduced directly: close first, rotate after.
func TestRotateIfExceeds_AfterCloseIsANoOp(t *testing.T) {
	dir := newTestDir(t)
	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := sp.Append(makeRecord("id-1", `{"name":"page_view"}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := sp.RotateIfExceeds(0); err != nil {
		t.Fatalf("RotateIfExceeds after Close = %v, want nil", err)
	}
	if err := sp.Rotate(); err != nil {
		t.Fatalf("Rotate after Close = %v, want nil", err)
	}

	// The closed spool must not have been rotated or reopened: the one record
	// is still in the active segment and no archive exists.
	records, err := spool.ReadRecords(spool.Path(dir))
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("active segment holds %d records, want 1", len(records))
	}
	archives, err := filepath.Glob(filepath.Join(dir, "ingest-*.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if len(archives) != 0 {
		t.Fatalf("a closed spool was rotated: %v", archives)
	}
}

func TestClose_Twice(t *testing.T) {
	dir := newTestDir(t)
	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := sp.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := sp.Close(); err != nil {
		t.Fatalf("second Close = %v, want nil", err)
	}
}

// An append after Close fails with ErrClosed, so a record is never reported
// written when it was not, and a failed rotation cannot reopen a closed spool.
func TestAppend_AfterCloseFails(t *testing.T) {
	dir := newTestDir(t)
	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := sp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := sp.Append(makeRecord("id-late", `{}`)); !errors.Is(err, spool.ErrClosed) {
		t.Fatalf("Append after Close = %v, want ErrClosed", err)
	}
	if err := sp.AppendBatch([]spool.Record{makeRecord("id-late-batch", `{}`)}); !errors.Is(err, spool.ErrClosed) {
		t.Fatalf("AppendBatch after Close = %v, want ErrClosed", err)
	}
	if _, err := os.Stat(spool.Path(dir)); err != nil {
		t.Fatalf("active segment missing: %v", err)
	}
}
