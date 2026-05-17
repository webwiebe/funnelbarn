package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// GetInstanceSetting retrieves a single setting by key.
// Returns ("", false, nil) when the key does not exist.
func (s *Store) GetInstanceSetting(ctx context.Context, key string) (string, bool, error) {
	const q = `SELECT value FROM instance_settings WHERE key = ?`
	var value string
	err := s.db.QueryRowContext(ctx, q, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// SetInstanceSetting upserts a setting value.
func (s *Store) SetInstanceSetting(ctx context.Context, key, value string) error {
	const q = `
		INSERT INTO instance_settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`
	_, err := s.db.ExecContext(ctx, q, key, value, time.Now().UTC())
	return err
}

// GetAllInstanceSettings returns all settings as a map.
func (s *Store) GetAllInstanceSettings(ctx context.Context) (map[string]string, error) {
	const q = `SELECT key, value FROM instance_settings ORDER BY key`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}
