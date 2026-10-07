package repository_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// updatePurge rewrites the golden file instead of comparing against it.
// Refresh with:  go test ./internal/repository -run TestPurgeRetention -update-purge
var updatePurge = flag.Bool("update-purge", false, "rewrite golden files in internal/repository/testdata/purge")

const oldRowsPerProject = 650 // 2 projects => 1300 old rows per table, > 1200

type purgeGolden struct {
	EventsDeleted      int64    `json:"events_deleted"`
	EvaluationsDeleted int64    `json:"evaluations_deleted"`
	EventsSecondRun    int64    `json:"events_second_run"`
	EvalsSecondRun     int64    `json:"evaluations_second_run"`
	EventsRemaining    []string `json:"events_remaining"`
	EvalsRemaining     []string `json:"evaluations_remaining"`
}

// TestPurgeRetention pins the retention semantics of PurgeOldEvents and
// PurgeOldEvaluations: rows strictly older than the cutoff go, rows at or
// after it stay, and a second run deletes nothing. Rows are seeded with raw
// SQL so the test does not depend on how RecordEvaluation writes.
func TestPurgeRetention(t *testing.T) {
	ctx := context.Background()
	s, err := repository.Open(filepath.Join(t.TempDir(), "purge.db"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	type offset struct {
		label string
		d     time.Duration
	}
	edges := []offset{
		{"minus1s", -time.Second},
		{"at", 0},
		{"plus1s", time.Second},
		{"new", 48 * time.Hour},
	}

	type seedProject struct{ id, flagID, name string }
	var projects []seedProject
	for _, name := range []string{"alpha", "beta"} {
		p, err := s.CreateProject(ctx, name, name)
		require.NoError(t, err)
		f, err := s.CreateFlag(ctx, repository.FeatureFlag{
			ProjectID: p.ID, FlagKey: "purge-" + name, Name: name, FlagType: "boolean",
			Variants: `{"on":true,"off":false}`, DefaultVariant: "off",
			Split: `{"on":50,"off":50}`, Status: "active",
		})
		require.NoError(t, err)
		projects = append(projects, seedProject{p.ID, f.ID, name})
	}

	tx, err := s.DB().BeginTx(ctx, nil)
	require.NoError(t, err)
	for _, p := range projects {
		times := map[string]time.Time{}
		for i := 0; i < oldRowsPerProject; i++ {
			times[fmt.Sprintf("old%04d", i)] = cutoff.Add(-time.Hour - time.Duration(i)*time.Minute)
		}
		for _, e := range edges {
			times[e.label] = cutoff.Add(e.d)
		}
		for label, ts := range times {
			id := p.name + "-" + label
			_, err := tx.ExecContext(ctx,
				`INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at) VALUES (?, ?, ?, ?, ?, ?)`,
				"ev-"+id, p.id, "sess-"+id, "page_view", "ing-"+id, ts)
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx,
				`INSERT INTO flag_evaluations (id, flag_id, project_id, variant, context_hash, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
				"fe-"+id, p.flagID, p.id, "on", "hash-"+id, ts)
			require.NoError(t, err)
		}
	}
	require.NoError(t, tx.Commit())

	var got purgeGolden
	got.EventsDeleted, err = s.PurgeOldEvents(ctx, cutoff)
	require.NoError(t, err)
	got.EvaluationsDeleted, err = s.PurgeOldEvaluations(ctx, cutoff)
	require.NoError(t, err)
	got.EventsSecondRun, err = s.PurgeOldEvents(ctx, cutoff)
	require.NoError(t, err)
	got.EvalsSecondRun, err = s.PurgeOldEvaluations(ctx, cutoff)
	require.NoError(t, err)
	require.Zero(t, got.EventsSecondRun)
	require.Zero(t, got.EvalsSecondRun)

	got.EventsRemaining = purgeIDs(t, s, "events")
	got.EvalsRemaining = purgeIDs(t, s, "flag_evaluations")

	out, err := json.MarshalIndent(got, "", "  ")
	require.NoError(t, err)
	out = append(out, '\n')

	path := filepath.Join("testdata", "purge", "retention.golden.json")
	if *updatePurge {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, out, 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden file; run with -update-purge")
	require.Equal(t, string(want), string(out))
}

func purgeIDs(t *testing.T, s *repository.Store, table string) []string {
	t.Helper()
	rows, err := s.DB().QueryContext(context.Background(), `SELECT id FROM `+table+` ORDER BY id`)
	require.NoError(t, err)
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}
