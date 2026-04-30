package spool

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// makeRecord returns a minimal valid Record for testing.
func makeRecord(id string) Record {
	return Record{
		IngestID:    id,
		ReceivedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ContentType: "application/json",
		RemoteAddr:  "127.0.0.1",
		BodyBase64:  base64.StdEncoding.EncodeToString([]byte(`{"event":"test"}`)),
		ProjectSlug: "test-project",
	}
}

// --- New / NewWithLimit ---

func TestNew_CreatesSpoolDir(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "spool")

	sp, err := New(subDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	if _, err := os.Stat(subDir); err != nil {
		t.Fatalf("expected dir to exist: %v", err)
	}
}

func TestNew_CreatesActiveFile(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	want := filepath.Join(dir, DefaultFileName)
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected active file to exist: %v", err)
	}
}

func TestNewWithLimit_ZeroIsUnlimited(t *testing.T) {
	dir := t.TempDir()
	sp, err := NewWithLimit(dir, 0)
	if err != nil {
		t.Fatalf("NewWithLimit: %v", err)
	}
	defer sp.Close()

	// Should accept many records without ErrFull.
	for i := 0; i < 100; i++ {
		if err := sp.Append(makeRecord("id")); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}
}

func TestNewWithLimit_ReturnsErrFullWhenExceeded(t *testing.T) {
	dir := t.TempDir()
	sp, err := NewWithLimit(dir, 10) // very small limit
	if err != nil {
		t.Fatalf("NewWithLimit: %v", err)
	}
	defer sp.Close()

	// The first append will exceed 10 bytes.
	err = sp.Append(makeRecord("id"))
	if !errors.Is(err, ErrFull) {
		t.Fatalf("expected ErrFull, got %v", err)
	}
}

// --- Path ---

func TestSpool_Path(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	want := filepath.Join(dir, DefaultFileName)
	if got := sp.Path(); got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

func TestSpool_PathNilSpool(t *testing.T) {
	var sp *Spool
	if got := sp.Path(); got != "" {
		t.Fatalf("nil Spool.Path() = %q, want empty string", got)
	}
}

func TestPath_Function(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, DefaultFileName)
	if got := Path(dir); got != want {
		t.Fatalf("Path(%q) = %q, want %q", dir, got, want)
	}
}

// --- Append ---

func TestAppend_SingleRecord(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	rec := makeRecord("abc-123")
	if err := sp.Append(rec); err != nil {
		t.Fatalf("Append: %v", err)
	}

	records, err := ReadRecords(sp.Path())
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].IngestID != "abc-123" {
		t.Fatalf("IngestID = %q, want abc-123", records[0].IngestID)
	}
}

func TestAppend_MultipleRecords(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	for i := 0; i < 5; i++ {
		id := string(rune('A' + i))
		if err := sp.Append(makeRecord(id)); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	records, err := ReadRecords(sp.Path())
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(records) != 5 {
		t.Fatalf("expected 5 records, got %d", len(records))
	}
}

func TestAppend_EmptyBodyBase64IsAccepted(t *testing.T) {
	// base64.StdEncoding.EncodeToString(nil) == "" so a record without a body
	// round-trips with an empty BodyBase64 — that is the current behaviour.
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	rec := Record{IngestID: "no-body", ReceivedAt: time.Now()}
	if err := sp.Append(rec); err != nil {
		t.Fatalf("Append: %v", err)
	}

	records, err := ReadRecords(sp.Path())
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record")
	}
	// The record is persisted correctly regardless of empty body.
	if records[0].IngestID != "no-body" {
		t.Fatalf("IngestID = %q, want no-body", records[0].IngestID)
	}
}

func TestAppend_NilSpool(t *testing.T) {
	var sp *Spool
	err := sp.Append(makeRecord("x"))
	if err == nil {
		t.Fatal("expected error for nil spool")
	}
}

// --- AppendBatch ---

func TestAppendBatch_MultipleRecords(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	batch := []Record{makeRecord("r1"), makeRecord("r2"), makeRecord("r3")}
	if err := sp.AppendBatch(batch); err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}

	records, err := ReadRecords(sp.Path())
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}
	ids := []string{records[0].IngestID, records[1].IngestID, records[2].IngestID}
	for i, want := range []string{"r1", "r2", "r3"} {
		if ids[i] != want {
			t.Fatalf("record[%d].IngestID = %q, want %q", i, ids[i], want)
		}
	}
}

func TestAppendBatch_EmptySliceIsNoop(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	if err := sp.AppendBatch(nil); err != nil {
		t.Fatalf("AppendBatch(nil): %v", err)
	}
	if err := sp.AppendBatch([]Record{}); err != nil {
		t.Fatalf("AppendBatch([]): %v", err)
	}

	records, err := ReadRecords(sp.Path())
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(records))
	}
}

func TestAppendBatch_ReturnsErrFullWhenExceeded(t *testing.T) {
	dir := t.TempDir()
	sp, err := NewWithLimit(dir, 10) // very small limit
	if err != nil {
		t.Fatalf("NewWithLimit: %v", err)
	}
	defer sp.Close()

	err = sp.AppendBatch([]Record{makeRecord("id")})
	if !errors.Is(err, ErrFull) {
		t.Fatalf("expected ErrFull, got %v", err)
	}
}

func TestAppendBatch_EmptyBodyBase64IsAccepted(t *testing.T) {
	// Same as the single-record case: nil body encodes to "" so BodyBase64
	// stays empty in persisted JSON. Verify the records are persisted at all.
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	batch := []Record{
		{IngestID: "b1", ReceivedAt: time.Now()},
		{IngestID: "b2", ReceivedAt: time.Now()},
	}
	if err := sp.AppendBatch(batch); err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}

	records, err := ReadRecords(sp.Path())
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	wantIDs := []string{"b1", "b2"}
	for i, r := range records {
		if r.IngestID != wantIDs[i] {
			t.Fatalf("records[%d].IngestID = %q, want %q", i, r.IngestID, wantIDs[i])
		}
	}
}

func TestAppendBatch_NilSpool(t *testing.T) {
	var sp *Spool
	err := sp.AppendBatch([]Record{makeRecord("x")})
	if err == nil {
		t.Fatal("expected error for nil spool")
	}
}

// --- ReadRecords ---

func TestReadRecords_NonExistentFileReturnsNil(t *testing.T) {
	records, err := ReadRecords("/no/such/file.ndjson")
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if records != nil {
		t.Fatalf("expected nil, got %v", records)
	}
}

func TestReadRecords_PreservesAllFields(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	rec := Record{
		IngestID:      "full-record",
		ReceivedAt:    time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC),
		ContentType:   "application/x-www-form-urlencoded",
		RemoteAddr:    "10.0.0.1",
		ContentLength: 42,
		BodyBase64:    base64.StdEncoding.EncodeToString([]byte("hello")),
		ProjectSlug:   "my-project",
	}
	if err := sp.Append(rec); err != nil {
		t.Fatalf("Append: %v", err)
	}

	records, err := ReadRecords(sp.Path())
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	got := records[0]
	if got.IngestID != rec.IngestID {
		t.Errorf("IngestID = %q, want %q", got.IngestID, rec.IngestID)
	}
	if !got.ReceivedAt.Equal(rec.ReceivedAt) {
		t.Errorf("ReceivedAt = %v, want %v", got.ReceivedAt, rec.ReceivedAt)
	}
	if got.ContentType != rec.ContentType {
		t.Errorf("ContentType = %q, want %q", got.ContentType, rec.ContentType)
	}
	if got.RemoteAddr != rec.RemoteAddr {
		t.Errorf("RemoteAddr = %q, want %q", got.RemoteAddr, rec.RemoteAddr)
	}
	if got.ContentLength != rec.ContentLength {
		t.Errorf("ContentLength = %d, want %d", got.ContentLength, rec.ContentLength)
	}
	if got.BodyBase64 != rec.BodyBase64 {
		t.Errorf("BodyBase64 = %q, want %q", got.BodyBase64, rec.BodyBase64)
	}
	if got.ProjectSlug != rec.ProjectSlug {
		t.Errorf("ProjectSlug = %q, want %q", got.ProjectSlug, rec.ProjectSlug)
	}
}

// --- ReadRecordsFrom ---

func TestReadRecordsFrom_FromZeroReadsAll(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	for _, id := range []string{"r1", "r2", "r3"} {
		if err := sp.Append(makeRecord(id)); err != nil {
			t.Fatalf("Append %s: %v", id, err)
		}
	}

	result, err := ReadRecordsFrom(sp.Path(), 0)
	if err != nil {
		t.Fatalf("ReadRecordsFrom: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 records, got %d", len(result))
	}
}

func TestReadRecordsFrom_NonZeroOffsetSkipsEarlierRecords(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	// Append first record and capture its end offset.
	if err := sp.Append(makeRecord("first")); err != nil {
		t.Fatalf("Append first: %v", err)
	}
	firstBatch, err := ReadRecordsFrom(sp.Path(), 0)
	if err != nil {
		t.Fatalf("ReadRecordsFrom: %v", err)
	}
	if len(firstBatch) != 1 {
		t.Fatalf("expected 1 record, got %d", len(firstBatch))
	}
	offsetAfterFirst := firstBatch[0].EndOffset

	// Append a second record.
	if err := sp.Append(makeRecord("second")); err != nil {
		t.Fatalf("Append second: %v", err)
	}

	// Read from offset — should only see the second record.
	result, err := ReadRecordsFrom(sp.Path(), offsetAfterFirst)
	if err != nil {
		t.Fatalf("ReadRecordsFrom with offset: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result))
	}
	if result[0].Record.IngestID != "second" {
		t.Fatalf("IngestID = %q, want second", result[0].Record.IngestID)
	}
}

func TestReadRecordsFrom_EndOffsetIsMonotonicallyIncreasing(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	for i := 0; i < 5; i++ {
		id := string(rune('A' + i))
		if err := sp.Append(makeRecord(id)); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	result, err := ReadRecordsFrom(sp.Path(), 0)
	if err != nil {
		t.Fatalf("ReadRecordsFrom: %v", err)
	}
	if len(result) != 5 {
		t.Fatalf("expected 5, got %d", len(result))
	}
	prev := int64(-1)
	for i, r := range result {
		if r.EndOffset <= prev {
			t.Fatalf("record[%d].EndOffset %d not > previous %d", i, r.EndOffset, prev)
		}
		prev = r.EndOffset
	}
}

func TestReadRecordsFrom_NonExistentFileReturnsNil(t *testing.T) {
	result, err := ReadRecordsFrom("/no/such/file.ndjson", 0)
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
}

// --- Cursor ---

func TestWriteAndReadCursor(t *testing.T) {
	dir := t.TempDir()

	if err := WriteCursor(dir, 12345); err != nil {
		t.Fatalf("WriteCursor: %v", err)
	}

	offset, err := ReadCursor(dir)
	if err != nil {
		t.Fatalf("ReadCursor: %v", err)
	}
	if offset != 12345 {
		t.Fatalf("ReadCursor = %d, want 12345", offset)
	}
}

func TestReadCursor_MissingFileReturnsZero(t *testing.T) {
	dir := t.TempDir()
	offset, err := ReadCursor(dir)
	if err != nil {
		t.Fatalf("ReadCursor: %v", err)
	}
	if offset != 0 {
		t.Fatalf("ReadCursor = %d, want 0 for missing file", offset)
	}
}

func TestResetCursor_RemovesCursorFile(t *testing.T) {
	dir := t.TempDir()

	if err := WriteCursor(dir, 999); err != nil {
		t.Fatalf("WriteCursor: %v", err)
	}

	if err := ResetCursor(dir); err != nil {
		t.Fatalf("ResetCursor: %v", err)
	}

	// After reset, cursor should be 0.
	offset, err := ReadCursor(dir)
	if err != nil {
		t.Fatalf("ReadCursor after reset: %v", err)
	}
	if offset != 0 {
		t.Fatalf("ReadCursor after reset = %d, want 0", offset)
	}
}

func TestResetCursor_IdempotentWhenNoCursor(t *testing.T) {
	dir := t.TempDir()
	// No cursor file present — should not error.
	if err := ResetCursor(dir); err != nil {
		t.Fatalf("ResetCursor on missing file: %v", err)
	}
}

func TestWriteCursor_OverwritesPreviousValue(t *testing.T) {
	dir := t.TempDir()

	if err := WriteCursor(dir, 100); err != nil {
		t.Fatalf("WriteCursor 100: %v", err)
	}
	if err := WriteCursor(dir, 200); err != nil {
		t.Fatalf("WriteCursor 200: %v", err)
	}

	offset, err := ReadCursor(dir)
	if err != nil {
		t.Fatalf("ReadCursor: %v", err)
	}
	if offset != 200 {
		t.Fatalf("ReadCursor = %d, want 200", offset)
	}
}

// --- Rotate ---

func TestRotate_RenamesActiveFile(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	if err := sp.Append(makeRecord("before-rotate")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	if err := sp.Rotate(); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	// Active file should now be empty (newly created).
	info, err := os.Stat(sp.Path())
	if err != nil {
		t.Fatalf("stat active file after rotate: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("active file size = %d after rotate, want 0", info.Size())
	}

	// There should be exactly one archived ingest-*.ndjson file.
	matches, err := filepath.Glob(filepath.Join(dir, "ingest-*.ndjson"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 archived file, got %d: %v", len(matches), matches)
	}
}

func TestRotate_ArchivedFileContainsPreviousRecords(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	if err := sp.Append(makeRecord("pre-rotate")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sp.Rotate(); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	archives, _ := filepath.Glob(filepath.Join(dir, "ingest-*.ndjson"))
	if len(archives) == 0 {
		t.Fatal("no archive file found")
	}
	records, err := ReadRecords(archives[0])
	if err != nil {
		t.Fatalf("ReadRecords archive: %v", err)
	}
	if len(records) != 1 || records[0].IngestID != "pre-rotate" {
		t.Fatalf("unexpected archived records: %v", records)
	}
}

func TestRotate_CanAppendAfterRotate(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	if err := sp.Rotate(); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if err := sp.Append(makeRecord("post-rotate")); err != nil {
		t.Fatalf("Append after rotate: %v", err)
	}

	records, err := ReadRecords(sp.Path())
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(records) != 1 || records[0].IngestID != "post-rotate" {
		t.Fatalf("unexpected records after rotate: %v", records)
	}
}

func TestRotate_NilSpool(t *testing.T) {
	var sp *Spool
	if err := sp.Rotate(); err == nil {
		t.Fatal("expected error for nil spool")
	}
}

// --- RotateIfExceeds ---

func TestRotateIfExceeds_BelowThresholdNoRotation(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	if err := sp.Append(makeRecord("small")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Very large threshold — should not rotate.
	if err := sp.RotateIfExceeds(1 << 20); err != nil {
		t.Fatalf("RotateIfExceeds: %v", err)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "ingest-*.ndjson"))
	if len(matches) != 0 {
		t.Fatalf("expected no archive, got %d", len(matches))
	}
}

func TestRotateIfExceeds_AboveThresholdRotates(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	for i := 0; i < 10; i++ {
		if err := sp.Append(makeRecord("fill")); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	// Threshold of 1 byte — anything causes rotation.
	if err := sp.RotateIfExceeds(1); err != nil {
		t.Fatalf("RotateIfExceeds: %v", err)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "ingest-*.ndjson"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 archive, got %d", len(matches))
	}
}

func TestRotateIfExceeds_NilSpool(t *testing.T) {
	var sp *Spool
	if err := sp.RotateIfExceeds(1000); err == nil {
		t.Fatal("expected error for nil spool")
	}
}

// --- RotateIfExceedsPath ---

func TestRotateIfExceedsPath_BelowThresholdNoOp(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := sp.Append(makeRecord("x")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	sp.Close()

	if err := RotateIfExceedsPath(dir, 1<<20); err != nil {
		t.Fatalf("RotateIfExceedsPath: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, DefaultFileName)); err != nil {
		t.Fatalf("active file should still exist: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "ingest-*.ndjson"))
	if len(matches) != 0 {
		t.Fatalf("expected no archive, got %d", len(matches))
	}
}

func TestRotateIfExceedsPath_AboveThresholdRenames(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for i := 0; i < 5; i++ {
		if err := sp.Append(makeRecord("x")); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	sp.Close()

	if err := RotateIfExceedsPath(dir, 1); err != nil {
		t.Fatalf("RotateIfExceedsPath: %v", err)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "ingest-*.ndjson"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 archived file, got %d", len(matches))
	}
}

func TestRotateIfExceedsPath_MissingFileIsNoop(t *testing.T) {
	dir := t.TempDir()
	// No spool file created.
	if err := RotateIfExceedsPath(dir, 1); err != nil {
		t.Fatalf("RotateIfExceedsPath on missing file: %v", err)
	}
}

// --- AppendDeadLetter ---

func TestAppendDeadLetter_WritesToDeadLetterFile(t *testing.T) {
	dir := t.TempDir()
	rec := makeRecord("dead-1")

	if err := AppendDeadLetter(dir, rec); err != nil {
		t.Fatalf("AppendDeadLetter: %v", err)
	}

	dlPath := filepath.Join(dir, deadLetterFileName)
	records, err := ReadRecords(dlPath)
	if err != nil {
		t.Fatalf("ReadRecords dead-letter: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 dead-letter record, got %d", len(records))
	}
	if records[0].IngestID != "dead-1" {
		t.Fatalf("dead-letter IngestID = %q, want dead-1", records[0].IngestID)
	}
}

func TestAppendDeadLetter_AppendsMultipleRecords(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 3; i++ {
		id := string(rune('a' + i))
		if err := AppendDeadLetter(dir, makeRecord(id)); err != nil {
			t.Fatalf("AppendDeadLetter %d: %v", i, err)
		}
	}

	dlPath := filepath.Join(dir, deadLetterFileName)
	records, err := ReadRecords(dlPath)
	if err != nil {
		t.Fatalf("ReadRecords dead-letter: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 dead-letter records, got %d", len(records))
	}
}

func TestAppendDeadLetter_DoesNotWriteToActiveSpool(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	if err := AppendDeadLetter(dir, makeRecord("oops")); err != nil {
		t.Fatalf("AppendDeadLetter: %v", err)
	}

	records, err := ReadRecords(sp.Path())
	if err != nil {
		t.Fatalf("ReadRecords active: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 records in active spool, got %d", len(records))
	}
}

// --- Round-trip scenarios ---

func TestRoundTrip_AppendReadCursorAdvance(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	ids := []string{"evt-1", "evt-2", "evt-3"}
	for _, id := range ids {
		if err := sp.Append(makeRecord(id)); err != nil {
			t.Fatalf("Append %s: %v", id, err)
		}
	}

	// Simulate a consumer: read from cursor, process, advance cursor.
	offset, err := ReadCursor(dir)
	if err != nil {
		t.Fatalf("ReadCursor: %v", err)
	}

	result, err := ReadRecordsFrom(sp.Path(), offset)
	if err != nil {
		t.Fatalf("ReadRecordsFrom: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 records, got %d", len(result))
	}

	// Advance cursor to after the second record.
	if err := WriteCursor(dir, result[1].EndOffset); err != nil {
		t.Fatalf("WriteCursor: %v", err)
	}

	// Restart: read cursor and replay from where we left off.
	offset2, err := ReadCursor(dir)
	if err != nil {
		t.Fatalf("ReadCursor 2: %v", err)
	}
	result2, err := ReadRecordsFrom(sp.Path(), offset2)
	if err != nil {
		t.Fatalf("ReadRecordsFrom 2: %v", err)
	}
	if len(result2) != 1 {
		t.Fatalf("expected 1 record after cursor advance, got %d", len(result2))
	}
	if result2[0].Record.IngestID != "evt-3" {
		t.Fatalf("expected evt-3, got %q", result2[0].Record.IngestID)
	}
}

func TestRoundTrip_BatchAppendAndBatchRead(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	batch1 := []Record{makeRecord("b1-r1"), makeRecord("b1-r2")}
	batch2 := []Record{makeRecord("b2-r1"), makeRecord("b2-r2"), makeRecord("b2-r3")}

	if err := sp.AppendBatch(batch1); err != nil {
		t.Fatalf("AppendBatch 1: %v", err)
	}
	if err := sp.AppendBatch(batch2); err != nil {
		t.Fatalf("AppendBatch 2: %v", err)
	}

	records, err := ReadRecords(sp.Path())
	if err != nil {
		t.Fatalf("ReadRecords: %v", err)
	}
	if len(records) != 5 {
		t.Fatalf("expected 5 records, got %d", len(records))
	}
	wantIDs := []string{"b1-r1", "b1-r2", "b2-r1", "b2-r2", "b2-r3"}
	for i, r := range records {
		if r.IngestID != wantIDs[i] {
			t.Errorf("records[%d].IngestID = %q, want %q", i, r.IngestID, wantIDs[i])
		}
	}
}

func TestRoundTrip_RotateAndContinueAppending(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	for _, id := range []string{"pre-1", "pre-2"} {
		if err := sp.Append(makeRecord(id)); err != nil {
			t.Fatalf("Append %s: %v", id, err)
		}
	}

	if err := sp.Rotate(); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	for _, id := range []string{"post-1", "post-2"} {
		if err := sp.Append(makeRecord(id)); err != nil {
			t.Fatalf("Append %s after rotate: %v", id, err)
		}
	}

	// Active file has only post-rotate records.
	active, err := ReadRecords(sp.Path())
	if err != nil {
		t.Fatalf("ReadRecords active: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("expected 2 active records, got %d", len(active))
	}

	// Archived file has pre-rotate records.
	archives, _ := filepath.Glob(filepath.Join(dir, "ingest-*.ndjson"))
	if len(archives) != 1 {
		t.Fatalf("expected 1 archive, got %d", len(archives))
	}
	archived, err := ReadRecords(archives[0])
	if err != nil {
		t.Fatalf("ReadRecords archive: %v", err)
	}
	if len(archived) != 2 {
		t.Fatalf("expected 2 archived records, got %d", len(archived))
	}
}

// --- NDJSON format integrity ---

func TestActiveFileIsValidNDJSON(t *testing.T) {
	dir := t.TempDir()
	sp, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()

	for i := 0; i < 3; i++ {
		if err := sp.Append(makeRecord("ndjson-check")); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	data, err := os.ReadFile(sp.Path())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	for i, line := range lines {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("line %d is not valid JSON: %v", i, err)
		}
	}
}
