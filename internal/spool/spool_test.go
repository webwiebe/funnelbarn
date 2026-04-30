package spool_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

func newTestDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "spool-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func makeRecord(id, body string) spool.Record {
	return spool.Record{
		IngestID:   id,
		ReceivedAt: time.Now().UTC().Truncate(time.Second),
		BodyBase64: base64.StdEncoding.EncodeToString([]byte(body)),
	}
}

// ---------------------------------------------------------------------------
// Append + ReadRecords round-trip
// ---------------------------------------------------------------------------

func TestAppend_ReadRecords(t *testing.T) {
	dir := newTestDir(t)
	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	r1 := makeRecord("id-1", `{"name":"page_view"}`)
	r2 := makeRecord("id-2", `{"name":"click"}`)

	if err := sp.Append(r1); err != nil {
		t.Fatalf("Append r1: %v", err)
	}
	if err := sp.Append(r2); err != nil {
		t.Fatalf("Append r2: %v", err)
	}

	records, err := spool.ReadRecords(spool.Path(dir))
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("want 2 records, got %d", len(records))
	}
	if records[0].IngestID != "id-1" || records[1].IngestID != "id-2" {
		t.Errorf("unexpected IDs: %v %v", records[0].IngestID, records[1].IngestID)
	}
}

// ---------------------------------------------------------------------------
// AppendBatch
// ---------------------------------------------------------------------------

func TestAppendBatch_ReadRecords(t *testing.T) {
	dir := newTestDir(t)
	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	batch := []spool.Record{
		makeRecord("b-1", `{"name":"a"}`),
		makeRecord("b-2", `{"name":"b"}`),
		makeRecord("b-3", `{"name":"c"}`),
	}
	if err := sp.AppendBatch(batch); err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}

	records, err := spool.ReadRecords(spool.Path(dir))
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("want 3 records, got %d", len(records))
	}
}

// ---------------------------------------------------------------------------
// Cursor: WriteCursor / ReadCursor / ResetCursor
// ---------------------------------------------------------------------------

func TestCursor_RoundTrip(t *testing.T) {
	dir := newTestDir(t)

	if err := spool.WriteCursor(dir, 12345); err != nil {
		t.Fatalf("WriteCursor: %v", err)
	}
	offset, err := spool.ReadCursor(dir)
	if err != nil {
		t.Fatalf("ReadCursor: %v", err)
	}
	if offset != 12345 {
		t.Errorf("want 12345, got %d", offset)
	}
}

func TestReadCursor_MissingFile(t *testing.T) {
	dir := newTestDir(t)
	offset, err := spool.ReadCursor(dir)
	if err != nil {
		t.Fatalf("ReadCursor on missing file: %v", err)
	}
	if offset != 0 {
		t.Errorf("want 0, got %d", offset)
	}
}

func TestResetCursor(t *testing.T) {
	dir := newTestDir(t)
	if err := spool.WriteCursor(dir, 99); err != nil {
		t.Fatalf("WriteCursor: %v", err)
	}
	if err := spool.ResetCursor(dir); err != nil {
		t.Fatalf("ResetCursor: %v", err)
	}
	offset, err := spool.ReadCursor(dir)
	if err != nil {
		t.Fatalf("ReadCursor after reset: %v", err)
	}
	if offset != 0 {
		t.Errorf("after reset, want 0, got %d", offset)
	}
}

// ---------------------------------------------------------------------------
// ReadRecordsFrom with byte offset (cursor-based replay)
// ---------------------------------------------------------------------------

func TestReadRecordsFrom_Offset(t *testing.T) {
	dir := newTestDir(t)
	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	r1 := makeRecord("r1", `{"name":"first"}`)
	r2 := makeRecord("r2", `{"name":"second"}`)

	if err := sp.Append(r1); err != nil {
		t.Fatalf("Append r1: %v", err)
	}

	// Read after first record to get the end offset.
	all, err := spool.ReadRecordsFrom(spool.Path(dir), 0)
	if err != nil {
		t.Fatalf("ReadRecordsFrom(0): %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("want 1 record, got %d", len(all))
	}
	offsetAfterFirst := all[0].EndOffset

	if err := sp.Append(r2); err != nil {
		t.Fatalf("Append r2: %v", err)
	}

	// Reading from the offset should yield only r2.
	remaining, err := spool.ReadRecordsFrom(spool.Path(dir), offsetAfterFirst)
	if err != nil {
		t.Fatalf("ReadRecordsFrom(offset): %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("want 1 record from offset, got %d", len(remaining))
	}
	if remaining[0].Record.IngestID != "r2" {
		t.Errorf("want r2, got %q", remaining[0].Record.IngestID)
	}
}

// ---------------------------------------------------------------------------
// ErrFull / size limit
// ---------------------------------------------------------------------------

func TestSizeLimit_ErrFull(t *testing.T) {
	dir := newTestDir(t)
	// Allow only 50 bytes — smaller than one record.
	sp, err := spool.NewWithLimit(dir, 50)
	if err != nil {
		t.Fatalf("NewWithLimit: %v", err)
	}
	defer sp.Close()

	r := makeRecord("overflow", `{"name":"big payload that exceeds the limit"}`)
	err = sp.Append(r)
	if err == nil {
		t.Fatal("expected ErrFull, got nil")
	}
	if err != spool.ErrFull {
		t.Errorf("want ErrFull, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Rotate
// ---------------------------------------------------------------------------

func TestRotate_CreatesArchiveFile(t *testing.T) {
	dir := newTestDir(t)
	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	if err := sp.Append(makeRecord("pre-rotate", `{"name":"x"}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}

	if err := sp.Rotate(); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	// After rotation, active spool should be empty.
	records, err := spool.ReadRecords(spool.Path(dir))
	if err != nil {
		t.Fatalf("ReadRecords after rotate: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("want 0 records after rotate, got %d", len(records))
	}

	// An archived file should exist.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var archived int
	for _, e := range entries {
		if e.Name() != spool.DefaultFileName && filepath.Ext(e.Name()) == ".ndjson" {
			archived++
		}
	}
	if archived == 0 {
		t.Error("expected at least one archived .ndjson file after rotate")
	}
}

// ---------------------------------------------------------------------------
// AppendDeadLetter
// ---------------------------------------------------------------------------

func TestAppendDeadLetter(t *testing.T) {
	dir := newTestDir(t)
	r := makeRecord("dead-1", `{"name":"bad"}`)
	if err := spool.AppendDeadLetter(dir, r); err != nil {
		t.Fatalf("AppendDeadLetter: %v", err)
	}
	dlPath := filepath.Join(dir, "deadletter.ndjson")
	if _, err := os.Stat(dlPath); err != nil {
		t.Fatalf("dead-letter file not created: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ReadRecords on non-existent path returns nil, not error
// ---------------------------------------------------------------------------

func TestReadRecords_MissingFile(t *testing.T) {
	records, err := spool.ReadRecords("/does/not/exist.ndjson")
	if err != nil {
		t.Fatalf("want nil error for missing file, got %v", err)
	}
	if records != nil {
		t.Errorf("want nil records for missing file, got %v", records)
	}
}

// ---------------------------------------------------------------------------
// Path method on struct vs package-level function
// ---------------------------------------------------------------------------

func TestSpoolPath_StructMethod(t *testing.T) {
	dir := newTestDir(t)
	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	p := sp.Path()
	if p == "" {
		t.Error("Spool.Path() should return non-empty path")
	}
	if !filepath.IsAbs(p) {
		t.Errorf("Spool.Path() should be absolute, got %q", p)
	}
}

func TestSpoolPath_NilSpool(t *testing.T) {
	var sp *spool.Spool
	if sp.Path() != "" {
		t.Error("nil Spool.Path() should return empty string")
	}
}

// ---------------------------------------------------------------------------
// RotateIfExceeds
// ---------------------------------------------------------------------------

func TestRotateIfExceeds_BelowThreshold(t *testing.T) {
	dir := newTestDir(t)
	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	if err := sp.Append(makeRecord("ri-1", `{"name":"x"}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// Threshold is very high — should NOT rotate.
	if err := sp.RotateIfExceeds(1 << 30); err != nil {
		t.Fatalf("RotateIfExceeds: %v", err)
	}
	// Active spool should still have records.
	records, err := spool.ReadRecords(spool.Path(dir))
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("want 1 record, got %d", len(records))
	}
}

func TestRotateIfExceeds_AboveThreshold(t *testing.T) {
	dir := newTestDir(t)
	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	if err := sp.Append(makeRecord("ri-big", `{"name":"x"}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// Threshold is 1 byte — always rotates.
	if err := sp.RotateIfExceeds(1); err != nil {
		t.Fatalf("RotateIfExceeds(1): %v", err)
	}
	// Active spool should now be empty.
	records, err := spool.ReadRecords(spool.Path(dir))
	if err != nil {
		t.Fatalf("ReadRecords after rotate: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("want 0 records after forced rotate, got %d", len(records))
	}
}

// ---------------------------------------------------------------------------
// RotateIfExceedsPath (package-level)
// ---------------------------------------------------------------------------

func TestRotateIfExceedsPath_NonExistentDir(t *testing.T) {
	// Missing spool file should not error.
	if err := spool.RotateIfExceedsPath("/does/not/exist", 100); err != nil {
		t.Errorf("want nil for missing path, got %v", err)
	}
}

func TestRotateIfExceedsPath_RotatesLargeFile(t *testing.T) {
	dir := newTestDir(t)
	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := sp.Append(makeRecord("big", `{"name":"x"}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	sp.Close()

	// Threshold of 1 byte should trigger rotation.
	if err := spool.RotateIfExceedsPath(dir, 1); err != nil {
		t.Fatalf("RotateIfExceedsPath: %v", err)
	}
	// Active spool file should be gone (renamed).
	records, _ := spool.ReadRecords(spool.Path(dir))
	if len(records) != 0 {
		t.Errorf("want 0 records after rotation, got %d", len(records))
	}
}

// ---------------------------------------------------------------------------
// AppendBatch size limit
// ---------------------------------------------------------------------------

func TestAppendBatch_SizeLimit(t *testing.T) {
	dir := newTestDir(t)
	sp, err := spool.NewWithLimit(dir, 50)
	if err != nil {
		t.Fatalf("NewWithLimit: %v", err)
	}
	defer sp.Close()

	batch := []spool.Record{
		makeRecord("b-big-1", `{"name":"event that is definitely larger than fifty bytes total"}`),
		makeRecord("b-big-2", `{"name":"second"}`),
	}
	err = sp.AppendBatch(batch)
	if err != spool.ErrFull {
		t.Errorf("want ErrFull for oversized batch, got %v", err)
	}
}
