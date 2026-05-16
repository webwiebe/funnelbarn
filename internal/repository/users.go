package repository

import (
	"context"
	"fmt"
	"time"
)

// User represents an admin user.
type User struct {
	ID           string
	Username     string
	PasswordHash string
	IAMBarnSub   string // empty when not linked to IAMBarn
	CreatedAt    time.Time
}

// UpsertUser inserts or updates a user by username.
func (s *Store) UpsertUser(ctx context.Context, username, passwordHash string) error {
	const q = `
		INSERT INTO users (id, username, password_hash)
		VALUES (?, ?, ?)
		ON CONFLICT(username) DO UPDATE SET password_hash = excluded.password_hash`
	_, err := s.db.ExecContext(ctx, q, newUUID(), username, passwordHash)
	return err
}

// UserByUsername fetches a user by username.
func (s *Store) UserByUsername(ctx context.Context, username string) (User, error) {
	const q = `SELECT id, username, password_hash, COALESCE(iambarn_sub, ''), created_at FROM users WHERE username = ?`
	var u User
	err := s.db.QueryRowContext(ctx, q, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IAMBarnSub, &u.CreatedAt)
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// FindUserByIAMBarnSub fetches a user by their IAMBarn subject identifier.
func (s *Store) FindUserByIAMBarnSub(ctx context.Context, sub string) (User, error) {
	const q = `SELECT id, username, password_hash, COALESCE(iambarn_sub, ''), created_at FROM users WHERE iambarn_sub = ?`
	var u User
	err := s.db.QueryRowContext(ctx, q, sub).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IAMBarnSub, &u.CreatedAt)
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// CreateIAMBarnUser upserts a user keyed by IAMBarn sub. password_hash is set
// to a sentinel so IAMBarn users cannot log in via password auth.
// If username conflicts with an existing local account, sub is used as username.
func (s *Store) CreateIAMBarnUser(ctx context.Context, sub, username string) (User, error) {
	// If sub is already in the DB, just update the display name.
	if _, err := s.FindUserByIAMBarnSub(ctx, sub); err == nil {
		const upd = `UPDATE users SET username = ? WHERE iambarn_sub = ?`
		_, _ = s.db.ExecContext(ctx, upd, username, sub)
		return s.FindUserByIAMBarnSub(ctx, sub)
	}

	const ins = `INSERT INTO users (id, username, password_hash, iambarn_sub) VALUES (?, ?, 'iambarn', ?)`
	if _, err := s.db.ExecContext(ctx, ins, newUUID(), username, sub); err != nil {
		// Username likely conflicts with a local account — fall back to using sub.
		if _, err2 := s.db.ExecContext(ctx, ins, newUUID(), sub, sub); err2 != nil {
			return User{}, fmt.Errorf("create iambarn user: %w", err)
		}
	}
	return s.FindUserByIAMBarnSub(ctx, sub)
}
