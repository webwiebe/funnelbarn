package spool_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

func recordLine(t *testing.T, r spool.Record) string {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return string(b) + "\n"
}

func writeSpool(t *testing.T, dir, content string) string {
	t.Helper()
	path := spool.Path(dir)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// ids lists the entries' ingest IDs, with "!" for a malformed entry.
func ids(entries []spool.RecordAtOffset) string {
	var out []string
	for _, e := range entries {
		if e.Malformed != nil {
			out = append(out, "!")
			continue
		}
		out = append(out, e.Record.IngestID)
	}
	return strings.Join(out, " ")
}

func readAll(t *testing.T, path string, offset int64) []spool.RecordAtOffset {
	t.Helper()
	got, err := spool.ReadRecordsFrom(path, offset)
	if err != nil {
		t.Fatalf("ReadRecordsFrom(%d): %v", offset, err)
	}
	return got
}

// TestReadRecordsFrom_SplicedLine reproduces the testing outage of 2026-10-04:
// a write torn mid-record, then the next record appended on the same line.
// The reader recovers the second record and steps past the torn bytes.
func TestReadRecordsFrom_SplicedLine(t *testing.T) {
	dir := newTestDir(t)
	r1 := recordLine(t, makeRecord("r1", `{"n":1}`))
	torn := `{"ingestId":"torn","receivedAt":"2026-10-04T14:14:29Z","userA`
	r2 := recordLine(t, makeRecord("r2", `{"n":2}`))
	r3 := recordLine(t, makeRecord("r3", `{"n":3}`))
	path := writeSpool(t, dir, r1+torn+r2+r3)

	got := readAll(t, path, 0)
	if want := "r1 ! r2 r3"; ids(got) != want {
		t.Fatalf("entries = %q, want %q", ids(got), want)
	}
	if want := int64(len(r1 + torn)); got[1].EndOffset != want {
		t.Errorf("malformed EndOffset = %d, want %d (start of the recovered record)", got[1].EndOffset, want)
	}
	if want := int64(len(r1 + torn + r2 + r3)); got[3].EndOffset != want {
		t.Errorf("last EndOffset = %d, want %d", got[3].EndOffset, want)
	}

	// A cursor saved after the torn bytes resumes at the recovered record.
	if got, want := ids(readAll(t, path, got[1].EndOffset)), "r2 r3"; got != want {
		t.Errorf("resumed entries = %q, want %q", got, want)
	}
}

func TestReadRecordsFrom_MalformedLineIsSkipped(t *testing.T) {
	dir := newTestDir(t)
	r1 := recordLine(t, makeRecord("r1", `{"n":1}`))
	r2 := recordLine(t, makeRecord("r2", `{"n":2}`))
	path := writeSpool(t, dir, r1+"not json\n"+r2)

	got := readAll(t, path, 0)
	if want := "r1 ! r2"; ids(got) != want {
		t.Fatalf("entries = %q, want %q", ids(got), want)
	}
	if want := int64(len(r1) + len("not json\n")); got[1].EndOffset != want {
		t.Errorf("malformed EndOffset = %d, want %d", got[1].EndOffset, want)
	}
}

// A final line without its newline may still be being written, so the reader
// leaves it for the next read. Consuming it would also put the cursor past
// EOF, which replays the whole file.
func TestReadRecordsFrom_LeavesUnterminatedTail(t *testing.T) {
	dir := newTestDir(t)
	r1 := recordLine(t, makeRecord("r1", `{"n":1}`))
	r2 := recordLine(t, makeRecord("r2", `{"n":2}`))
	path := writeSpool(t, dir, r1+strings.TrimSuffix(r2, "\n"))

	got := readAll(t, path, 0)
	if want := "r1"; ids(got) != want {
		t.Fatalf("entries = %q, want %q", ids(got), want)
	}
	if got[0].EndOffset != int64(len(r1)) {
		t.Errorf("EndOffset = %d, want %d", got[0].EndOffset, len(r1))
	}
}

// bufio.Scanner stopped at 64 KiB lines; a large record must still be read.
func TestReadRecordsFrom_LongRecord(t *testing.T) {
	dir := newTestDir(t)
	path := writeSpool(t, dir, recordLine(t, makeRecord("big", strings.Repeat("x", 200_000))))

	if got, want := ids(readAll(t, path, 0)), "big"; got != want {
		t.Fatalf("entries = %q, want %q", got, want)
	}
}

// Opening a spool whose last write was torn ends that line first, so the next
// append starts a line of its own.
func TestNew_TerminatesTornTail(t *testing.T) {
	dir := newTestDir(t)
	r1 := recordLine(t, makeRecord("r1", `{"n":1}`))
	torn := `{"ingestId":"torn","receivedAt":"2026`
	writeSpool(t, dir, r1+torn)

	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()
	if err := sp.Append(makeRecord("r2", `{"n":2}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got := readAll(t, spool.Path(dir), 0)
	if want := "r1 ! r2"; ids(got) != want {
		t.Fatalf("entries = %q, want %q", ids(got), want)
	}
	if want := int64(len(r1+torn) + 1); got[1].EndOffset != want {
		t.Errorf("malformed EndOffset = %d, want %d", got[1].EndOffset, want)
	}
}

func TestNew_LeavesCleanFileAlone(t *testing.T) {
	dir := newTestDir(t)
	r1 := recordLine(t, makeRecord("r1", `{"n":1}`))
	writeSpool(t, dir, r1)

	sp, err := spool.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sp.Close()
	size, err := spool.ActiveSize(dir)
	if err != nil {
		t.Fatalf("ActiveSize: %v", err)
	}
	if size != int64(len(r1)) {
		t.Errorf("size = %d, want %d", size, len(r1))
	}
}

func TestReadRecordsFrom_ReadErrorIsReturned(t *testing.T) {
	// A directory opens but cannot be read as a file.
	if _, err := spool.ReadRecordsFrom(newTestDir(t), 0); err == nil {
		t.Fatal("want an error reading a directory")
	}
}
