package repository

import (
	"context"
	"fmt"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// UpsertUser inserts or updates a user by username.
func (s *Store) UpsertUser(ctx context.Context, username, passwordHash string) error {
	const q = `
		INSERT INTO users (id, username, password_hash)
		VALUES (?, ?, ?)
		ON CONFLICT(username) DO UPDATE SET password_hash = excluded.password_hash`
	id, err := generateUUID()
	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}
	_, err = s.db.ExecContext(ctx, q, id, username, passwordHash)
	return err
}

// CountUsers returns the number of user rows. Used at startup to decide whether
// local/DB-user authentication is a configured login mechanism (so the API can
// fail closed rather than serve routes unauthenticated).
func (s *ReadStore) CountUsers(ctx context.Context) (int, error) {
	var n int
	if err := s.rdb.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// UserByUsername fetches a user by username.
func (s *ReadStore) UserByUsername(ctx context.Context, username string) (domain.User, error) {
	const q = `SELECT id, username, password_hash, COALESCE(iambarn_sub, ''), created_at FROM users WHERE username = ?`
	var u domain.User
	err := s.rdb.QueryRowContext(ctx, q, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IAMBarnSub, &u.CreatedAt)
	if err != nil {
		return domain.User{}, err
	}
	return u, nil
}

// FindUserByIAMBarnSub fetches a user by their IAMBarn subject identifier.
func (s *ReadStore) FindUserByIAMBarnSub(ctx context.Context, sub string) (domain.User, error) {
	return s.findUserByIAMBarnSub(ctx, s.rdb, sub)
}

func (s *ReadStore) findUserByIAMBarnSub(ctx context.Context, db querier, sub string) (domain.User, error) {
	const q = `SELECT id, username, password_hash, COALESCE(iambarn_sub, ''), created_at FROM users WHERE iambarn_sub = ?`
	var u domain.User
	err := db.QueryRowContext(ctx, q, sub).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IAMBarnSub, &u.CreatedAt)
	if err != nil {
		return domain.User{}, err
	}
	return u, nil
}

// CreateIAMBarnUser upserts a user keyed by IAMBarn sub. password_hash is set
// to a sentinel so IAMBarn users cannot log in via password auth.
// If username conflicts with an existing local account, sub is used as username.
func (s *Store) CreateIAMBarnUser(ctx context.Context, sub, username string) (domain.User, error) {
	// If sub is already in the DB, just update the display name.
	if _, err := s.findUserByIAMBarnSub(ctx, s.db, sub); err == nil {
		const upd = `UPDATE users SET username = ? WHERE iambarn_sub = ?`
		if _, updErr := s.db.ExecContext(ctx, upd, username, sub); updErr != nil {
			return domain.User{}, fmt.Errorf("update iambarn user display name: %w", updErr)
		}
		return s.findUserByIAMBarnSub(ctx, s.db, sub)
	}

	const ins = `INSERT INTO users (id, username, password_hash, iambarn_sub) VALUES (?, ?, 'iambarn', ?)`
	id, err := generateUUID()
	if err != nil {
		return domain.User{}, fmt.Errorf("create iambarn user: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, ins, id, username, sub); err != nil {
		// Username likely conflicts with a local account — fall back to using sub.
		// The first insert failed, so its id is still free.
		if _, err2 := s.db.ExecContext(ctx, ins, id, sub, sub); err2 != nil {
			return domain.User{}, fmt.Errorf("create iambarn user: %w", err)
		}
	}
	return s.findUserByIAMBarnSub(ctx, s.db, sub)
}
