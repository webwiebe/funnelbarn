package command

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultSpoolFallbackMaxBytes caps a fallback file when
	// NewSpoolFallback gets zero. A bookkeeping command encodes to a few hundred
	// bytes, so this holds days of evaluate traffic.
	DefaultSpoolFallbackMaxBytes int64 = 64 << 20
	// spoolFallbackBatch bounds one publish of buffered commands.
	spoolFallbackBatch = 500
)

// SpoolFallback is the fallback bus of a reader process, which has no write
// connection to apply a command to. Submit appends the encoded command to
// <dir>/<name>.ndjson; Forward publishes what the file holds once Valkey takes
// publishes again, and moves a cursor past it only after the publish
// succeeded. A crash between the publish and the cursor write sends a command
// twice, which is safe because every command is idempotent.
type SpoolFallback struct {
	path       string
	cursorPath string
	maxBytes   int64
	logger     *slog.Logger

	mu   sync.Mutex
	file *os.File
	full bool // the file reached maxBytes; logged once per episode
}

var _ Bus = (*SpoolFallback)(nil)

// NewSpoolFallback opens (or creates) the fallback file name in dir. maxBytes
// zero means DefaultSpoolFallbackMaxBytes.
func NewSpoolFallback(dir, name string, maxBytes int64, logger *slog.Logger) (*SpoolFallback, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultSpoolFallbackMaxBytes
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f := &SpoolFallback{
		path:       filepath.Join(dir, name+".ndjson"),
		cursorPath: filepath.Join(dir, name+".cursor"),
		maxBytes:   maxBytes,
		logger:     logger,
	}
	file, err := os.OpenFile(f.path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	if err := terminateTornLine(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	f.file = file
	return f, nil
}

// terminateTornLine ends the file with a newline when a crash cut the last
// write short, so the next command starts on a line of its own. The torn line
// fails to decode at the consumer, which drops it.
func terminateTornLine(file *os.File) error {
	info, err := file.Stat()
	if err != nil || info.Size() == 0 {
		return err
	}
	last := make([]byte, 1)
	if _, err := file.ReadAt(last, info.Size()-1); err != nil {
		return err
	}
	if last[0] == '\n' {
		return nil
	}
	_, err = file.Write([]byte{'\n'})
	return err
}

// Start implements Bus. There is no consumer.
func (f *SpoolFallback) Start(context.Context) {}

// Submit appends c to the file. At the size cap it drops c and logs at Error,
// because the disk would fill otherwise.
func (f *SpoolFallback) Submit(ctx context.Context, c Command) time.Duration {
	start := time.Now()
	payload, err := Encode(c, start)
	if err != nil {
		f.logger.ErrorContext(ctx, "spool fallback encode failed, command not applied", "kind", c.Kind(), "error", err, "handled", true)
		return time.Since(start)
	}
	payload = append(payload, '\n')

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.file == nil {
		f.logger.ErrorContext(ctx, "command submitted to closed spool fallback, not applied", "kind", c.Kind(), "handled", true)
		return time.Since(start)
	}
	info, err := f.file.Stat()
	if err == nil && info.Size()+int64(len(payload)) > f.maxBytes {
		if !f.full {
			f.full = true
			f.logger.ErrorContext(ctx, "spool fallback full, dropping commands until Valkey takes them again",
				"handled", false, "file", filepath.Base(f.path), "max_bytes", f.maxBytes)
		}
		return time.Since(start)
	}
	if _, err := f.file.Write(payload); err != nil {
		f.logger.ErrorContext(ctx, "spool fallback write failed, command not applied", "kind", c.Kind(), "error", err, "handled", false)
	}
	return time.Since(start)
}

// Flush implements Bus. Commands in the file wait for Forward, so there is
// nothing in this process to wait for.
func (f *SpoolFallback) Flush(context.Context) error { return nil }

// Close closes the file. What it holds is forwarded after the next start.
func (f *SpoolFallback) Close(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.file == nil {
		return nil
	}
	err := f.file.Close()
	f.file = nil
	return err
}

// Forward publishes the buffered commands in batches and returns how many it
// published. It stops at the first failed publish and returns its error; the
// commands from there on stay in the file for the next call. Once everything
// is published it empties the file.
func (f *SpoolFallback) Forward(ctx context.Context, publish func(context.Context, [][]byte) error) (int, error) {
	offset, err := f.readCursor()
	if err != nil {
		return 0, err
	}
	lines, err := f.readFrom(offset)
	if err != nil {
		return 0, err
	}
	sent := 0
	for len(lines) > 0 {
		n := min(len(lines), spoolFallbackBatch)
		batch := make([][]byte, 0, n)
		next := offset
		for _, l := range lines[:n] {
			next += l.size
			if len(l.payload) > 0 {
				batch = append(batch, l.payload)
			}
		}
		if len(batch) > 0 {
			if err := publish(ctx, batch); err != nil {
				return sent, err
			}
		}
		offset = next
		if err := f.writeCursor(offset); err != nil {
			return sent, err
		}
		sent += len(batch)
		lines = lines[n:]
	}
	if err := f.truncateIfDrained(offset); err != nil {
		return sent, err
	}
	return sent, nil
}

// spoolLine is one complete line of the fallback file: its payload without the
// newline (empty for a blank line) and its size on disk.
type spoolLine struct {
	payload []byte
	size    int64
}

// readFrom returns the complete lines after offset. A line still being
// written has no newline yet and is left for the next call.
func (f *SpoolFallback) readFrom(offset int64) ([]spoolLine, error) {
	file, err := os.Open(f.path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }() // read-only; a close error loses nothing
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if offset > info.Size() {
		// The file was emptied by hand; nothing after the cursor is left.
		return nil, f.writeCursor(0)
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	var lines []spoolLine
	r := bufio.NewReader(file)
	for {
		line, err := r.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			return lines, nil
		}
		if err != nil {
			return nil, err
		}
		lines = append(lines, spoolLine{
			payload: bytes.TrimSuffix(line, []byte{'\n'}),
			size:    int64(len(line)),
		})
	}
}

// truncateIfDrained empties the file once the cursor has reached its end, so
// it does not grow without bound. A Submit that raced in after readFrom keeps
// the file at its new size and waits for the next Forward.
func (f *SpoolFallback) truncateIfDrained(offset int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.file == nil {
		return nil
	}
	info, err := f.file.Stat()
	if err != nil {
		return err
	}
	if info.Size() != offset || offset == 0 {
		return nil
	}
	// Cursor first: a crash before the truncate then sends the file again,
	// which is safe, where the other order would leave the cursor past the end.
	if err := f.writeCursor(0); err != nil {
		return err
	}
	if err := f.file.Truncate(0); err != nil {
		return err
	}
	if f.full {
		f.full = false
		f.logger.Info("spool fallback drained", "file", filepath.Base(f.path))
	}
	return nil
}

func (f *SpoolFallback) readCursor() (int64, error) {
	b, err := os.ReadFile(f.cursorPath)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("spool fallback cursor %s: %w", filepath.Base(f.cursorPath), err)
	}
	return v, nil
}

// writeCursor replaces the cursor file atomically.
func (f *SpoolFallback) writeCursor(offset int64) error {
	tmp := f.cursorPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.FormatInt(offset, 10)), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.cursorPath)
}
