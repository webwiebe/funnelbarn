package repository_test

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// seedProject inserts one row into every table that carries a project_id. The
// tag keeps primary keys apart between projects; flagKey, sessionID and
// rawName are the keys two projects may share.
func seedProject(t *testing.T, db *sql.DB, projectID, tag, flagKey, sessionID, rawName string) {
	t.Helper()
	stmts := []struct {
		q    string
		args []any
	}{
		{`INSERT OR IGNORE INTO canonical_events (key, label) VALUES ('page_view', 'Page view')`, nil},
		{`INSERT INTO events (id, project_id, session_id, name, ingest_id, occurred_at, environment) VALUES (?, ?, ?, 'page_view', ?, '2026-11-01 10:00:00', 'production')`,
			[]any{"ev-" + tag, projectID, sessionID, "ing-" + tag}},
		{`INSERT INTO sessions (id, project_id, first_seen_at, last_seen_at, event_count, environment) VALUES (?, ?, '2026-11-01 10:00:00', '2026-11-01 10:05:00', 3, 'production')`,
			[]any{sessionID, projectID}},
		{`INSERT INTO recordings (id, project_id, session_id, started_at, environment) VALUES (?, ?, ?, '2026-11-01 10:00:00', 'production')`,
			[]any{"rec-" + tag, projectID, sessionID}},
		{`INSERT INTO recording_traces (project_id, session_id, recording_id, trace_id, occurred_at) VALUES (?, ?, ?, ?, '2026-11-01 10:00:00')`,
			[]any{projectID, sessionID, "rec-" + tag, "trace-" + tag}},
		{`INSERT INTO funnels (id, project_id, name) VALUES (?, ?, ?)`, []any{"fun-" + tag, projectID, "Funnel " + tag}},
		{`INSERT INTO segments (id, project_id, name) VALUES (?, ?, ?)`, []any{"seg-" + tag, projectID, "Segment " + tag}},
		{`INSERT INTO ab_tests (id, project_id, name, conversion_event) VALUES (?, ?, ?, 'signup')`, []any{"ab-" + tag, projectID, "Test " + tag}},
		{`INSERT INTO dashboard_widgets (id, project_id, event_name, property) VALUES (?, ?, 'signup', 'plan')`, []any{"wid-" + tag, projectID}},
		{`INSERT INTO feature_flags (id, project_id, flag_key, name) VALUES (?, ?, ?, ?)`, []any{"flag-" + tag, projectID, flagKey, "Flag " + tag}},
		{`INSERT INTO flag_evaluations (id, flag_id, project_id, variant, context_hash) VALUES (?, ?, ?, 'on', 'h')`,
			[]any{"eval-" + tag, "flag-" + tag, projectID}},
		{`INSERT INTO event_name_mappings (project_id, raw_name, canonical_key) VALUES (?, ?, 'page_view')`, []any{projectID, rawName}},
		{`INSERT INTO api_keys (id, project_id, name, key_hash) VALUES (?, ?, ?, ?)`, []any{"key-" + tag, projectID, "Key " + tag, "hash-" + tag}},
		{`INSERT INTO project_health (project_id) VALUES (?)`, []any{projectID}},
		{`INSERT INTO project_recording_settings (project_id) VALUES (?)`, []any{projectID}},
	}
	for _, s := range stmts {
		_, err := db.Exec(s.q, s.args...)
		require.NoError(t, err, s.q)
	}
}

// tableRows returns every row of table with project_id removed, one string per
// row, sorted. Comparing it before and after a merge asserts that no other
// column changed.
func tableRows(t *testing.T, db *sql.DB, table string, projectIDs ...string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT * FROM ` + table)
	require.NoError(t, err)
	defer rows.Close()
	cols, err := rows.Columns()
	require.NoError(t, err)
	var out []string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		require.NoError(t, rows.Scan(ptrs...))
		var parts []string
		for i, c := range cols {
			if c == "project_id" {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s=%v", c, vals[i]))
		}
		out = append(out, strings.Join(parts, "|"))
	}
	require.NoError(t, rows.Err())
	sort.Strings(out)
	return out
}

func countByProject(t *testing.T, db *sql.DB, table, projectID string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE project_id = ?`, projectID).Scan(&n))
	return n
}

// TestMergeProjects_ListsEveryTableWithAProjectID fails when a migration adds a
// table with a project_id column that the merge does not know about.
func TestMergeProjects_ListsEveryTableWithAProjectID(t *testing.T) {
	s := newTestStore(t)
	rows, err := s.DB().Query(`SELECT m.name FROM sqlite_master m, pragma_table_info(m.name) p
		WHERE m.type = 'table' AND p.name = 'project_id' ORDER BY m.name`)
	require.NoError(t, err)
	defer rows.Close()
	var live []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		live = append(live, n)
	}
	require.NoError(t, rows.Err())

	from, to := mustProject(t, s, "src"), mustProject(t, s, "dst")
	report, err := s.MergeProjects(context.Background(), from, to, "", true)
	require.NoError(t, err)
	var merged []string
	for _, c := range report.Tables {
		merged = append(merged, c.Table)
	}
	sort.Strings(merged)
	require.Equal(t, live, merged, "every table with project_id must be moved by MergeProjects")
}

func mustProject(t *testing.T, s *repository.Store, slug string) domain.Project {
	t.Helper()
	p, err := s.CreateProject(context.Background(), slug, slug)
	require.NoError(t, err)
	return p
}

func TestMergeProjects_MovesEveryTableAndKeepsEveryColumn(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	db := s.DB()
	from, to := mustProject(t, s, "src"), mustProject(t, s, "dst")
	seedProject(t, db, from.ID, "from", "flag-from", "sess-from", "raw-from")
	seedProject(t, db, to.ID, "to", "flag-to", "sess-to", "raw-to")
	// Singletons exist once per project, so the target keeps its own.
	_, err := db.Exec(`DELETE FROM project_health WHERE project_id = ?`, to.ID)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM project_recording_settings WHERE project_id = ?`, to.ID)
	require.NoError(t, err)

	before := map[string][]string{}
	for _, table := range []string{"events", "sessions", "recordings", "recording_traces", "funnels", "segments",
		"ab_tests", "dashboard_widgets", "feature_flags", "flag_evaluations", "event_name_mappings",
		"api_keys", "project_health", "project_recording_settings"} {
		before[table] = tableRows(t, db, table)
	}

	report, err := s.MergeProjects(ctx, from, to, "production", false)
	require.NoError(t, err)
	require.False(t, report.DryRun)
	for _, c := range report.Tables {
		require.EqualValues(t, 1, c.Moved, c.Table)
		require.EqualValues(t, 0, c.Left, c.Table)
		require.Equal(t, 0, countByProject(t, db, c.Table, from.ID), c.Table)
		if c.Table == "project_health" || c.Table == "project_recording_settings" {
			require.Equal(t, 1, countByProject(t, db, c.Table, to.ID), c.Table)
			continue
		}
		require.Equal(t, 2, countByProject(t, db, c.Table, to.ID), c.Table)
	}

	// Per column: the merge changed project_id and nothing else. The key
	// environment is the one deliberate change, so api_keys is compared without it.
	for table, want := range before {
		got := tableRows(t, db, table)
		if table == "api_keys" {
			for i := range got {
				got[i] = strings.Replace(got[i], "environment=production", "environment=", 1)
			}
			sort.Strings(got)
		}
		require.Equal(t, want, got, table)
	}
	var env, targetEnv string
	require.NoError(t, db.QueryRow(`SELECT environment FROM api_keys WHERE id = 'key-from'`).Scan(&env))
	require.Equal(t, "production", env)
	// The target's own key keeps the environment it had.
	require.NoError(t, db.QueryRow(`SELECT environment FROM api_keys WHERE id = 'key-to'`).Scan(&targetEnv))
	require.Equal(t, "", targetEnv)

	got, err := s.ProjectByID(ctx, from.ID)
	require.NoError(t, err)
	require.Equal(t, repository.ProjectArchivedStatus, got.Status)
	got, err = s.ProjectByID(ctx, to.ID)
	require.NoError(t, err)
	require.Equal(t, "active", got.Status)
}

func TestMergeProjects_LeavesRowsTheTargetAlreadyHolds(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	db := s.DB()
	from, to := mustProject(t, s, "src"), mustProject(t, s, "dst")
	// Same flag key, session id and raw name in both projects.
	seedProject(t, db, from.ID, "from", "docs_public", "sess-shared", "raw-shared")
	seedProject(t, db, to.ID, "to", "docs_public", "sess-shared", "raw-shared")

	report, err := s.MergeProjects(ctx, from, to, "", false)
	require.NoError(t, err)

	left := map[string]int64{}
	for _, c := range report.Tables {
		left[c.Table] = c.Left
	}
	require.EqualValues(t, 1, left["feature_flags"])
	require.EqualValues(t, 1, left["sessions"])
	require.EqualValues(t, 1, left["event_name_mappings"])
	require.EqualValues(t, 1, left["project_health"])
	require.EqualValues(t, 1, left["project_recording_settings"])
	require.EqualValues(t, 0, left["events"])

	// Nothing is deleted: the colliding rows stay with the archived source.
	require.Equal(t, 1, countByProject(t, db, "feature_flags", from.ID))
	require.Equal(t, 1, countByProject(t, db, "feature_flags", to.ID))
	require.Equal(t, 2, countByProject(t, db, "events", to.ID))
}

func TestMergeProjects_DryRunChangesNothingAndReportsTheRealCounts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	db := s.DB()
	from, to := mustProject(t, s, "src"), mustProject(t, s, "dst")
	seedProject(t, db, from.ID, "from", "flag-from", "sess-from", "raw-from")

	report, err := s.MergeProjects(ctx, from, to, "production", true)
	require.NoError(t, err)
	require.True(t, report.DryRun)
	require.EqualValues(t, 1, report.Tables[0].Moved)

	require.Equal(t, 1, countByProject(t, db, "events", from.ID))
	require.Equal(t, 0, countByProject(t, db, "events", to.ID))
	got, err := s.ProjectByID(ctx, from.ID)
	require.NoError(t, err)
	require.Equal(t, "active", got.Status)
	var env string
	require.NoError(t, db.QueryRow(`SELECT environment FROM api_keys WHERE id = 'key-from'`).Scan(&env))
	require.Equal(t, "", env)
}

func TestMergeProjects_RepointsCanonicalFunnelsOnce(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	db := s.DB()
	from, to := mustProject(t, s, "src"), mustProject(t, s, "dst")
	_, err := db.Exec(`INSERT INTO canonical_funnels (id, name, project_ids) VALUES
		('cf1', 'only source', ?), ('cf2', 'both', ?), ('cf3', 'neither', '[]')`,
		`["`+from.ID+`"]`, `["`+from.ID+`","`+to.ID+`"]`)
	require.NoError(t, err)

	report, err := s.MergeProjects(ctx, from, to, "", false)
	require.NoError(t, err)
	require.EqualValues(t, 2, report.CanonicalFunnels)

	want := map[string]string{"cf1": `["` + to.ID + `"]`, "cf2": `["` + to.ID + `"]`, "cf3": `[]`}
	for id, ids := range want {
		var got string
		require.NoError(t, db.QueryRow(`SELECT project_ids FROM canonical_funnels WHERE id = ?`, id).Scan(&got))
		require.Equal(t, ids, got, id)
	}
}

func TestMergeProjects_RefusesTheSameProject(t *testing.T) {
	s := newTestStore(t)
	p := mustProject(t, s, "solo")
	_, err := s.MergeProjects(context.Background(), p, p, "", false)
	require.ErrorIs(t, err, domain.ErrValidation)
}
