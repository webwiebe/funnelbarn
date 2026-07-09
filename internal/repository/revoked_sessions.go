package repository

import (
	"context"
	"time"
)

// RevokedSession is a session token hash that has been invalidated (logout)
// together with the moment it would have expired on its own.
type RevokedSession struct {
	TokenHash string
	ExpiresAt time.Time
}

// RevokeSession records a session token hash as revoked until expiresAt. It is
// idempotent — revoking the same token twice is a no-op.
func (s *Store) RevokeSession(ctx context.Context, tokenHash string, expiresAt time.Time) error {
	const q = `INSERT INTO revoked_sessions (token_hash, expires_at) VALUES (?, ?)
		ON CONFLICT(token_hash) DO NOTHING`
	_, err := s.db.ExecContext(ctx, q, tokenHash, expiresAt.Unix())
	return err
}

// ActiveRevokedSessions returns revocations that have not yet expired as of now.
// Used at startup to repopulate the in-memory revocation set so a logout
// survives a process restart.
func (s *Store) ActiveRevokedSessions(ctx context.Context, now time.Time) ([]RevokedSession, error) {
	const q = `SELECT token_hash, expires_at FROM revoked_sessions WHERE expires_at > ?`
	rows, err := s.db.QueryContext(ctx, q, now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RevokedSession
	for rows.Next() {
		var r RevokedSession
		var exp int64
		if err := rows.Scan(&r.TokenHash, &exp); err != nil {
			return nil, err
		}
		r.ExpiresAt = time.Unix(exp, 0).UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteExpiredRevokedSessions prunes revocation rows whose tokens have already
// expired (they can never be presented again, so keeping them wastes space).
func (s *Store) DeleteExpiredRevokedSessions(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM revoked_sessions WHERE expires_at <= ?`, now.Unix())
	return err
}
