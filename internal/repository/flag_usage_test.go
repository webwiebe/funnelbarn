package repository

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

func openFlagUsageStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "usage.db"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestFlagUsageDailyCountsPerFlagDayAndVariantInsideTheWindow(t *testing.T) {
	ctx := context.Background()
	s := openFlagUsageStore(t)
	p, err := s.CreateProject(ctx, "Usage", "usage")
	require.NoError(t, err)
	other, err := s.CreateProject(ctx, "Other", "usage-other")
	require.NoError(t, err)
	mk := func(proj, key string) string {
		f, err := s.CreateFlag(ctx, domain.FeatureFlag{ProjectID: proj, FlagKey: key, Name: key, FlagType: "boolean",
			Variants: `{"on":true,"off":false}`, DefaultVariant: "off", Split: `{"on":50,"off":50}`, Status: "active"})
		require.NoError(t, err)
		return f.ID
	}
	flagA, flagB, flagOther := mk(p.ID, "a"), mk(p.ID, "b"), mk(other.ID, "c")

	ins := func(proj, flag, variant, at string) {
		_, err := s.DB().ExecContext(ctx,
			`INSERT INTO flag_evaluations (id, flag_id, project_id, variant, context_hash, created_at) VALUES (lower(hex(randomblob(8))), ?, ?, ?, 'h', ?)`,
			flag, proj, variant, at)
		require.NoError(t, err)
	}
	ins(p.ID, flagA, "on", "2026-10-14 08:00:00")
	ins(p.ID, flagA, "on", "2026-10-14 23:59:59")
	ins(p.ID, flagA, "off", "2026-10-14 12:00:00")
	ins(p.ID, flagA, "on", "2026-10-15 00:00:00")
	ins(p.ID, flagB, "on", "2026-10-15 09:00:00")
	ins(p.ID, flagA, "on", "2026-10-13 23:59:59") // before the window
	ins(p.ID, flagA, "on", "2026-10-16 00:00:00") // at the exclusive end
	ins(other.ID, flagOther, "on", "2026-10-14 10:00:00")

	from := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	rows, err := s.FlagUsageDaily(ctx, p.ID, from, to)
	require.NoError(t, err)

	got := map[string]int64{}
	for _, r := range rows {
		got[r.FlagID+"|"+r.Day+"|"+r.Variant] = r.Count
	}
	assert.Equal(t, map[string]int64{
		flagA + "|2026-10-14|on":  2,
		flagA + "|2026-10-14|off": 1,
		flagA + "|2026-10-15|on":  1,
		flagB + "|2026-10-15|on":  1,
	}, got)
}

// A 30 day aggregate that scanned the whole table would be fine on a test
// database and a stall in production. The plan is the contract, not a timing:
// it must search idx_flag_evals_project_time on project_id and created_at.
func TestFlagUsageDailyUsesTheProjectTimeIndex(t *testing.T) {
	ctx := context.Background()
	s := openFlagUsageStore(t)

	rows, err := s.DB().QueryContext(ctx, "EXPLAIN QUERY PLAN "+flagUsageDailyQuery, "p", "2026-09-16 00:00:00", "2026-10-16 00:00:00")
	require.NoError(t, err)
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &notused, &detail))
		plan = append(plan, detail)
	}
	require.NoError(t, rows.Err())
	t.Log("EXPLAIN QUERY PLAN:\n" + strings.Join(plan, "\n"))

	joined := strings.Join(plan, "\n")
	assert.Contains(t, joined, "SEARCH flag_evaluations USING INDEX idx_flag_evals_project_time (project_id=? AND created_at>? AND created_at<?)")
	assert.NotContains(t, joined, "SCAN flag_evaluations")
}
