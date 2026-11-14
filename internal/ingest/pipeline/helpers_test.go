package pipeline

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
)

// levelCounts swaps the default logger for the test and returns a function
// that counts its lines by level whose message contains msg.
func levelCounts(t *testing.T) func(level, msg string) int {
	t.Helper()
	var (
		mu  sync.Mutex
		buf bytes.Buffer
	)
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(lockedWriter{&mu, &buf}, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return func(level, msg string) int {
		mu.Lock()
		defer mu.Unlock()
		n := 0
		for _, line := range bytes.Split(buf.Bytes(), []byte("\n")) {
			var rec map[string]any
			if json.Unmarshal(line, &rec) != nil {
				continue
			}
			if m, _ := rec["msg"].(string); rec["level"] == level && bytes.Contains([]byte(m), []byte(msg)) {
				n++
			}
		}
		return n
	}
}

type lockedWriter struct {
	mu *sync.Mutex
	w  *bytes.Buffer
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
