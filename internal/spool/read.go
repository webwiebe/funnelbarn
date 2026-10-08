package spool

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
)

// terminateTornTail ends the file with a newline when the last write was cut
// off (a pod killed mid-append). Without it the next record is appended to the
// torn one, and the spliced line fails to parse.
func terminateTornTail(path string, file *os.File) error {
	r, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	fi, err := r.Stat()
	if err != nil || fi.Size() == 0 {
		return err
	}
	last := make([]byte, 1)
	if _, err := r.ReadAt(last, fi.Size()-1); err != nil {
		return err
	}
	if last[0] == '\n' {
		return nil
	}
	_, err = file.Write([]byte{'\n'})
	return err
}

// RecordAtOffset pairs a Record with the byte offset after this record.
type RecordAtOffset struct {
	Record    Record
	EndOffset int64
	// Malformed is set when the bytes up to EndOffset are not a record, for
	// example a write torn by a crash. Record is then empty; the consumer
	// reports the bytes and moves its cursor past them.
	Malformed error
}

// recordStart opens every serialised Record (IngestID is its first field).
// JSON escapes quotes inside string values, so it cannot occur mid-record.
var recordStart = []byte(`{"ingestId":`)

// ReadRecordsFrom reads records from path starting at the given byte offset.
// A line that does not parse comes back as an entry with Malformed set, so one
// bad line cannot stop the reader. A final line without its newline is left
// for the next read: it may still be being written.
func ReadRecordsFrom(path string, offset int64) ([]RecordAtOffset, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()

	// Guard against a stale cursor after rotation: if the persisted offset is
	// past the end of the active file, the file was rotated (renamed to an
	// archive and recreated) or truncated since the cursor was written, so the
	// offset points beyond the new, shorter file. Seeking there would silently
	// read zero records forever and the consumer would never advance again.
	// Restart from the beginning — the records in the active file are post-rotation
	// and have not been processed yet, so this replays exactly the backlog.
	if offset > 0 {
		if fi, statErr := file.Stat(); statErr == nil && offset > fi.Size() {
			offset = 0
		}
	}

	if offset > 0 {
		if _, err := file.Seek(offset, 0); err != nil {
			return nil, err
		}
	}

	var records []RecordAtOffset
	pos := offset
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		start := pos
		pos += int64(len(line))
		line = line[:len(line)-1]
		if len(line) == 0 {
			continue
		}
		records = append(records, parseLine(line, start, pos)...)
	}
	return records, nil
}

// parseLine decodes one spool line that spans [start, end). When a torn write
// was followed by a new append, the line holds the torn bytes and then a whole
// record; the record is recovered and only the torn prefix is malformed.
func parseLine(line []byte, start, end int64) []RecordAtOffset {
	var record Record
	err := json.Unmarshal(line, &record)
	if err == nil {
		return []RecordAtOffset{{Record: record, EndOffset: end}}
	}
	if k := bytes.LastIndex(line, recordStart); k > 0 {
		var tail Record
		if json.Unmarshal(line[k:], &tail) == nil {
			return []RecordAtOffset{
				{EndOffset: start + int64(k), Malformed: err},
				{Record: tail, EndOffset: end},
			}
		}
	}
	return []RecordAtOffset{{EndOffset: end, Malformed: err}}
}
