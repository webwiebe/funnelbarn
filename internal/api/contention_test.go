package api

// Contention regression (issue #302). Production had one SQLite connection:
// the daily retention DELETE held it for ~11s and POST /api/v1/evaluate waited
// 4-6s behind it.
//
// TestEvaluateWithWriterHeld is the gate. It holds the only write connection
// for the whole test, so any write or read the evaluate path still does on the
// write pool blocks the request until the deadline. It passes or fails on
// that, independent of how fast or loaded the machine is.
//
// TestContention runs the real purge while evaluate requests arrive and
// asserts the spec's p99 < 100ms and that no row is lost. A wall-clock bound
// flakes on the shared self-hosted runners, so it only runs when
// FUNNELBARN_CONTENTION_TEST=1, on a quiet machine:
//
//	FUNNELBARN_CONTENTION_TEST=1 go test ./internal/api -run TestContention -count=1 -v
//
// After each deploy, scripts/verify-testing.sh measures the same p99 on the
// testing environment.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

const (
	contentionOldEvents      = 50000
	contentionOldEvaluations = 20000
	contentionMinSamples     = 20
	contentionP99Limit       = 100 * time.Millisecond
	heldWriterRequests       = 50
	heldWriterDeadline       = 10 * time.Second
	contentionRetentionDays  = 30
)

// seedContentionRows bulk-inserts rows older than the retention window in one
// transaction so the purge has real work to do.
func seedContentionRows(t *testing.T, store *repository.Store, seed *goldenSeed) {
	t.Helper()
	ctx := context.Background()
	old := time.Now().UTC().AddDate(0, 0, -contentionRetentionDays-60)
	tx, err := store.DB().BeginTx(ctx, nil)
	require.NoError(t, err)
	ev, err := tx.PrepareContext(ctx,
		`INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at) VALUES (?, ?, ?, ?, ?, ?)`)
	require.NoError(t, err)
	for i := 0; i < contentionOldEvents; i++ {
		ts := old.Add(-time.Duration(i) * time.Second)
		_, err = ev.ExecContext(ctx, fmt.Sprintf("cev-%06d", i), seed.ProjectA,
			fmt.Sprintf("csess-%06d", i%500), "page_view", fmt.Sprintf("cing-%06d", i), ts)
		require.NoError(t, err)
	}
	require.NoError(t, ev.Close())
	fe, err := tx.PrepareContext(ctx,
		`INSERT INTO flag_evaluations (id, flag_id, project_id, variant, context_hash, created_at) VALUES (?, ?, ?, ?, ?, ?)`)
	require.NoError(t, err)
	for i := 0; i < contentionOldEvaluations; i++ {
		ts := old.Add(-time.Duration(i) * time.Second)
		_, err = fe.ExecContext(ctx, fmt.Sprintf("cfe-%06d", i), seed.FlagExperiment, seed.ProjectA,
			"control", fmt.Sprintf("chash-%06d", i), ts)
		require.NoError(t, err)
	}
	require.NoError(t, fe.Close())
	require.NoError(t, tx.Commit())
}

func contentionEvaluate(handler http.Handler, key string, i int) (time.Duration, int) {
	body, _ := json.Marshal(map[string]any{
		"flag_key":      "checkout_redesign",
		"default_value": false,
		"context": map[string]any{
			"targeting_key": fmt.Sprintf("contention-u-%d", i),
			"session_id":    fmt.Sprintf("contention-%d", i),
			"country":       "NL",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evaluate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderAPIKey, key)
	rec := httptest.NewRecorder()
	start := time.Now()
	handler.ServeHTTP(rec, req)
	return time.Since(start), rec.Code
}

func TestContention(t *testing.T) {
	if os.Getenv("FUNNELBARN_CONTENTION_TEST") != "1" {
		t.Skip("wall-clock latency test; set FUNNELBARN_CONTENTION_TEST=1 on a quiet machine")
	}
	tgt, seed, deps := newGoldenEnv(t, defaultGoldenServer)
	store := deps.Store
	seedContentionRows(t, store, seed)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The same two calls runMaintenance makes for the retention step.
	cutoff := time.Now().AddDate(0, 0, -contentionRetentionDays)
	purgeDone := make(chan error, 1)
	go func() {
		if _, err := store.PurgeOldEvents(ctx, cutoff); err != nil {
			purgeDone <- err
			return
		}
		_, err := store.PurgeOldEvaluations(ctx, cutoff)
		purgeDone <- err
	}()

	var (
		samples []time.Duration
		okCount int
	)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
loop:
	for i := 0; ; i++ {
		select {
		case err := <-purgeDone:
			require.NoError(t, err)
			break loop
		case <-ctx.Done():
			t.Fatalf("purge did not finish within 30s")
		case <-ticker.C:
			d, code := contentionEvaluate(tgt.Handler, seed.KeyAIngest, i)
			require.Equal(t, http.StatusOK, code, "evaluate request %d", i)
			samples = append(samples, d)
			okCount++
		}
	}

	require.GreaterOrEqual(t, len(samples), contentionMinSamples,
		"purge finished before enough evaluate samples were taken; seed more rows")
	sort.Slice(samples, func(a, b int) bool { return samples[a] < samples[b] })
	p99 := samples[(len(samples)*99+99)/100-1]
	t.Logf("%d evaluate samples during purge, p50=%v p99=%v max=%v",
		len(samples), samples[len(samples)/2], p99, samples[len(samples)-1])
	require.Less(t, p99, contentionP99Limit, "evaluate p99 during purge")

	// Every accepted evaluation must reach the database once the queue drains.
	require.NotNil(t, tgt.Flush, "server under test has no dispatcher")
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer flushCancel()
	require.NoError(t, tgt.Flush(flushCtx))
	var landed int
	require.NoError(t, store.ReadDB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM flag_evaluations WHERE session_id LIKE 'contention-%'`).Scan(&landed))
	require.Equal(t, okCount, landed, "evaluation rows landed after Flush")
}

// TestEvaluateWithWriterHeld checks that evaluate (API key auth, known flags,
// an unknown key that auto-registers) completes while another holder keeps the
// single write connection, and that every queued row lands once it is released.
func TestEvaluateWithWriterHeld(t *testing.T) {
	tgt, seed, deps := newGoldenEnv(t, defaultGoldenServer)
	store := deps.Store
	require.NotNil(t, tgt.Flush, "server under test has no dispatcher")

	conn, err := store.DB().Conn(context.Background())
	require.NoError(t, err)
	released := false
	release := func() {
		if !released {
			released = true
			require.NoError(t, conn.Close())
		}
	}
	defer release()

	for i := 0; i < heldWriterRequests; i++ {
		done := make(chan int, 1)
		go func(i int) {
			_, code := contentionEvaluate(tgt.Handler, seed.KeyAIngest, i)
			done <- code
		}(i)
		select {
		case code := <-done:
			require.Equal(t, http.StatusOK, code, "evaluate request %d", i)
		case <-time.After(heldWriterDeadline):
			release() // let the blocked request finish before the test ends
			t.Fatalf("evaluate request %d waited on the write connection", i)
		}
	}

	// An unknown key queues EnsureAutoFlag instead of inserting inline.
	body := []byte(`{"flag_key":"held_writer_new_flag","default_value":false,"context":{"session_id":"contention-new"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/evaluate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderAPIKey, seed.KeyAIngest)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		tgt.Handler.ServeHTTP(rec, req)
		done <- rec
	}()
	select {
	case rec := <-done:
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	case <-time.After(heldWriterDeadline):
		release()
		t.Fatal("auto-registering evaluate waited on the write connection")
	}

	release()
	flushCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, tgt.Flush(flushCtx))

	var landed int
	require.NoError(t, store.ReadDB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM flag_evaluations WHERE session_id LIKE 'contention-%'`).Scan(&landed))
	require.Equal(t, heldWriterRequests, landed, "evaluation rows landed after Flush")
	_, err = store.FlagByKey(context.Background(), seed.ProjectA, "held_writer_new_flag")
	require.NoError(t, err, "auto-registered flag landed after Flush")
}
