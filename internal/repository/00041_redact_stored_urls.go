package repository

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pressly/goose/v3"

	"github.com/wiebe-xyz/funnelbarn/internal/enrich"
)

// 00041 replaces the values of sensitive URL parameters (token, invite, code,
// state, secret, password and their variants) in rows stored before ingest
// redacted them (#461, #463). The rows stay, so counts do not change; only the
// parameter values become enrich.RedactedValue, the same form ingest writes.
// It is a Go migration so that it applies enrich.RedactURL, the one definition
// of what counts as sensitive, instead of a second copy in SQL.
//
// The migration is idempotent: a row whose URLs are already redacted is left
// as it is. Replay chunks in object storage are outside the database and
// outside this migration.
func init() {
	goose.AddMigrationContext(up00041RedactStoredURLs, nil)
	goMigrationVersions = append(goMigrationVersions, 41)
}

// storedURLTable names the URL-bearing columns of one table. JSONCols hold a
// JSON object whose string values may be URLs.
type storedURLTable struct {
	Table    string
	Cols     []string
	JSONCols []string
}

var storedURLTables = []storedURLTable{
	{Table: "events", Cols: []string{"url", "referrer"}, JSONCols: []string{"properties"}},
	{Table: "sessions", Cols: []string{"entry_url", "exit_url", "referrer"}},
	{Table: "recordings", Cols: []string{"page_url"}},
	{Table: "recording_traces", Cols: []string{"url"}},
}

// storedURLNeedles pre-filter rows in SQL (LIKE ignores ASCII case) so only
// rows that can hold a sensitive parameter are read. enrich.RedactURL decides
// which of them change.
var storedURLNeedles = []string{"%token%", "%secret%", "%password%", "%invite=%", "%code=%", "%state=%"}

func up00041RedactStoredURLs(ctx context.Context, tx *sql.Tx) error {
	for _, t := range storedURLTables {
		if err := redactStoredURLTable(ctx, tx, t); err != nil {
			return fmt.Errorf("redact %s: %w", t.Table, err)
		}
	}
	return nil
}

type storedURLUpdate struct {
	rowid int64
	set   map[string]string
}

func redactStoredURLTable(ctx context.Context, tx *sql.Tx, t storedURLTable) error {
	cols := append(append([]string{}, t.Cols...), t.JSONCols...)
	updates, err := collectStoredURLUpdates(ctx, tx, t, cols)
	if err != nil {
		return err
	}
	return applyStoredURLUpdates(ctx, tx, t.Table, cols, updates)
}

// collectStoredURLUpdates reads the candidate rows of one table and returns
// the changes enrich.RedactURL makes to them.
func collectStoredURLUpdates(ctx context.Context, tx *sql.Tx, t storedURLTable, cols []string) ([]storedURLUpdate, error) {
	var where []string
	var args []any
	for _, c := range cols {
		for _, n := range storedURLNeedles {
			where = append(where, c+" LIKE ?")
			args = append(args, n)
		}
	}
	query := fmt.Sprintf("SELECT rowid, %s FROM %s WHERE %s", strings.Join(cols, ", "), t.Table, strings.Join(where, " OR "))

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var updates []storedURLUpdate
	for rows.Next() {
		var rowid int64
		vals := make([]sql.NullString, len(cols))
		dest := make([]any, 0, len(cols)+1)
		dest = append(dest, &rowid)
		for i := range vals {
			dest = append(dest, &vals[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		if set := redactedColumns(vals, cols, len(t.Cols)); len(set) > 0 {
			updates = append(updates, storedURLUpdate{rowid: rowid, set: set})
		}
	}
	return updates, rows.Err()
}

// redactedColumns returns the new value of every column that redaction
// changes. The first plain columns are URLs; the rest hold JSON objects.
func redactedColumns(vals []sql.NullString, cols []string, plain int) map[string]string {
	set := map[string]string{}
	for i, c := range cols {
		if !vals[i].Valid {
			continue
		}
		var next string
		var changed bool
		if i < plain {
			next = enrich.RedactURL(vals[i].String)
			changed = next != vals[i].String
		} else {
			next, changed = redactJSONObject(vals[i].String)
		}
		if changed {
			set[c] = next
		}
	}
	return set
}

func applyStoredURLUpdates(ctx context.Context, tx *sql.Tx, table string, cols []string, updates []storedURLUpdate) error {
	for _, u := range updates {
		var sets []string
		var uargs []any
		for _, c := range cols {
			if v, ok := u.set[c]; ok {
				sets = append(sets, c+" = ?")
				uargs = append(uargs, v)
			}
		}
		uargs = append(uargs, u.rowid)
		stmt := fmt.Sprintf("UPDATE %s SET %s WHERE rowid = ?", table, strings.Join(sets, ", "))
		if _, err := tx.ExecContext(ctx, stmt, uargs...); err != nil {
			return err
		}
	}
	return nil
}

// redactJSONObject redacts the URL-shaped string values of a JSON object and
// returns the re-encoded text. A value that is not a JSON object, or has
// nothing to redact, comes back unchanged with false.
func redactJSONObject(s string) (string, bool) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v map[string]any
	if err := dec.Decode(&v); err != nil || v == nil {
		return s, false
	}
	before, err := json.Marshal(v)
	if err != nil {
		return s, false
	}
	enrich.RedactURLStrings(v)
	after, err := json.Marshal(v)
	if err != nil || bytes.Equal(before, after) {
		return s, false
	}
	return string(after), true
}
