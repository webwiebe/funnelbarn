package repository

import (
	"context"
	"fmt"
)

// DistributionEntry is one value in a field distribution breakdown.
type DistributionEntry struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
	Pct   int    `json:"pct"`
}

// SessionDistributions returns project-wide value distributions for the fields
// used by the segment system: device_type, country_code, connection_class,
// dark_mode, browser_timezone (all from sessions), plus browser and os from events.
func (s *Store) SessionDistributions(ctx context.Context, projectID string) (map[string][]DistributionEntry, error) {
	result := make(map[string][]DistributionEntry)

	// Fields on the sessions table.
	sessionFields := []struct{ key, col string }{
		{"device_type", "device_type"},
		{"country_code", "country_code"},
		{"connection_class", "connection_class"},
		{"dark_mode", "CAST(dark_mode AS TEXT)"},
		{"browser_timezone", "browser_timezone"},
	}
	for _, f := range sessionFields {
		entries, err := s.sessionColDistribution(ctx, "sessions", projectID, f.col, f.key)
		if err != nil {
			continue
		}
		result[f.key] = entries
	}

	// browser and os live on the events table (sampled — first 10k events in window).
	for _, col := range []string{"browser", "os"} {
		entries, err := s.sessionColDistribution(ctx, "events", projectID, col, col)
		if err != nil {
			continue
		}
		result[col] = entries
	}

	return result, nil
}

func (s *Store) sessionColDistribution(ctx context.Context, table, projectID, col, key string) ([]DistributionEntry, error) {
	// Friendly label for dark_mode values.
	var labelExpr string
	if key == "dark_mode" {
		labelExpr = `CASE WHEN ` + col + ` = '1' THEN 'dark' WHEN ` + col + ` = '0' THEN 'light' ELSE ` + col + ` END`
	} else {
		labelExpr = col
	}

	q := fmt.Sprintf(`
		WITH total AS (
			SELECT COUNT(*) AS n
			FROM %s
			WHERE project_id = ?
			  AND %s IS NOT NULL
			  AND %s != ''
		)
		SELECT %s AS value,
		       COUNT(*) AS cnt,
		       CAST(ROUND(COUNT(*) * 100.0 / NULLIF(t.n, 0)) AS INTEGER) AS pct
		FROM %s, total t
		WHERE project_id = ?
		  AND %s IS NOT NULL
		  AND %s != ''
		GROUP BY %s
		ORDER BY cnt DESC
		LIMIT 10`,
		table, col, col,
		labelExpr,
		table, col, col, col,
	)

	rows, err := s.db.QueryContext(ctx, q, projectID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []DistributionEntry
	for rows.Next() {
		var e DistributionEntry
		if err := rows.Scan(&e.Value, &e.Count, &e.Pct); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
