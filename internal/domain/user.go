package domain

import "time"

// User represents an admin user.
type User struct {
	ID           string
	Username     string
	PasswordHash string
	IAMBarnSub   string // empty when not linked to IAMBarn
	CreatedAt    time.Time
}
