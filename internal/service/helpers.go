package service

import "strings"

// isUniqueConstraint reports whether err is a SQLite UNIQUE constraint violation.
func isUniqueConstraint(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
