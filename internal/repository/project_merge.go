package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// ProjectArchivedStatus is the projects.status of a project whose data was
// merged into another project. Its rows that could not move stay behind.
const ProjectArchivedStatus = "archived"

// mergeTables lists every table with a project_id column. A test compares it
// with the live schema, so a table added later fails until it is listed here.
var mergeTables = []string{
	"events",
	"sessions",
	"recordings",
	"recording_traces",
	"funnels",
	"segments",
	"ab_tests",
	"dashboard_widgets",
	"feature_flags",
	"flag_evaluations",
	"event_name_mappings",
	"api_keys",
	"project_health",
	"project_recording_settings",
}

// MergeTableCount is the outcome for one table of a project merge.
type MergeTableCount struct {
	Table string `json:"table"`
	// Moved rows now belong to the target project.
	Moved int64 `json:"moved"`
	// Left rows stay with the source because the target already holds a row
	// with the same key (a flag key, a session id, a per-project setting).
	Left int64 `json:"left"`
}

// MergeReport is what MergeProjects did, or would do when DryRun is set.
type MergeReport struct {
	From   string            `json:"from"`
	To     string            `json:"to"`
	Env    string            `json:"env,omitempty"`
	DryRun bool              `json:"dry_run"`
	Tables []MergeTableCount `json:"tables"`
	// CanonicalFunnels is the number of cross-project funnels whose project
	// list named the source and now names the target.
	CanonicalFunnels int64 `json:"canonical_funnels"`
}

// MergeProjects moves the data of project from into project to in one
// transaction. Nothing is deleted. A row whose key the target already holds
// stays with the source, and the report counts it as left. The source is then
// marked archived. Its API keys move to the target, and env, when set, becomes
// the environment those keys file their events under. With dryRun the
// transaction is rolled back after counting, so the report is exact.
func (s *Store) MergeProjects(ctx context.Context, from, to domain.Project, env string, dryRun bool) (MergeReport, error) {
	report := MergeReport{From: from.Slug, To: to.Slug, Env: env, DryRun: dryRun}
	if from.ID == "" || to.ID == "" || from.ID == to.ID {
		return report, fmt.Errorf("merge projects: %w: source and target must be two different projects", domain.ErrValidation)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return report, fmt.Errorf("merge projects: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, table := range mergeTables {
		count, err := moveTable(ctx, tx, table, from.ID, to.ID)
		if err != nil {
			return report, fmt.Errorf("merge projects: %s: %w", table, err)
		}
		report.Tables = append(report.Tables, count)
	}
	if report.CanonicalFunnels, err = repointCanonicalFunnels(ctx, tx, from.ID, to.ID); err != nil {
		return report, fmt.Errorf("merge projects: canonical funnels: %w", err)
	}
	if env != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE api_keys SET environment = ? WHERE project_id = ?`, env, to.ID); err != nil {
			return report, fmt.Errorf("merge projects: key environment: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE projects SET status = ? WHERE id = ?`, ProjectArchivedStatus, from.ID); err != nil {
		return report, fmt.Errorf("merge projects: archive source: %w", err)
	}
	if dryRun {
		return report, nil
	}
	if err := tx.Commit(); err != nil {
		return report, fmt.Errorf("merge projects: commit: %w", err)
	}
	return report, nil
}

// moveTable repoints a table's rows from one project to another. UPDATE OR
// IGNORE skips a row that would break a unique key of the target.
func moveTable(ctx context.Context, tx *sql.Tx, table, from, to string) (MergeTableCount, error) {
	count := MergeTableCount{Table: table}
	res, err := tx.ExecContext(ctx, `UPDATE OR IGNORE `+table+` SET project_id = ? WHERE project_id = ?`, to, from)
	if err != nil {
		return count, err
	}
	if count.Moved, err = res.RowsAffected(); err != nil {
		return count, err
	}
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE project_id = ?`, from).Scan(&count.Left)
	return count, err
}

// repointCanonicalFunnels replaces the source id with the target id in the
// JSON project lists of cross-project funnels, once per list.
func repointCanonicalFunnels(ctx context.Context, tx *sql.Tx, from, to string) (int64, error) {
	res, err := tx.ExecContext(ctx, `
UPDATE canonical_funnels
SET project_ids = (
    SELECT json_group_array(DISTINCT CASE WHEN j.value = ?1 THEN ?2 ELSE j.value END)
    FROM json_each(canonical_funnels.project_ids) j)
WHERE EXISTS (SELECT 1 FROM json_each(canonical_funnels.project_ids) j WHERE j.value = ?1)`, from, to)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// FormatMergeReport renders the report as one line per table.
func FormatMergeReport(r MergeReport) string {
	var b strings.Builder
	mode := "merged"
	if r.DryRun {
		mode = "would merge"
	}
	fmt.Fprintf(&b, "%s %s into %s", mode, r.From, r.To)
	if r.Env != "" {
		fmt.Fprintf(&b, " (keys file under environment %s)", r.Env)
	}
	b.WriteString("\n")
	for _, t := range r.Tables {
		fmt.Fprintf(&b, "  %-28s moved %d, left %d\n", t.Table, t.Moved, t.Left)
	}
	fmt.Fprintf(&b, "  %-28s repointed %d\n", "canonical_funnels", r.CanonicalFunnels)
	return b.String()
}
