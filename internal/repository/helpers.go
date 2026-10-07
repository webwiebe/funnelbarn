package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
)

// querier is the read surface shared by *sql.DB handles. Methods that read
// back their own write take the write pool explicitly; plain reads take the
// read pool.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// generateUUID returns a random UUID v4 string.
func generateUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func newUUID() string {
	id, _ := generateUUID()
	return id
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
