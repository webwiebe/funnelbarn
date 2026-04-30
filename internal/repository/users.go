package repository

import (
	"context"
	"time"
)

// User represents an admin user.
type User struct {
	ID           string
	Username     string
	PasswordHash string
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
	const q = `SELECT id, username, password_hash, created_at FROM users WHERE username = ?`
	var u User
	err := s.db.QueryRowContext(ctx, q, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return User{}, err
	}
	return u, nil
}
