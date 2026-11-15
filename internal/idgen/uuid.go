// Package idgen generates the random identifiers rows are keyed by.
package idgen

import (
	"crypto/rand"
	"fmt"
	"io"
)

// UUID returns a random UUID v4 string read from crypto/rand.
func UUID() (string, error) {
	return UUIDFrom(rand.Reader)
}

// UUIDFrom returns a UUID v4 string read from src. Tests pass a failing
// reader to check that a create path returns the error.
func UUIDFrom(src io.Reader) (string, error) {
	var b [16]byte
	if _, err := io.ReadFull(src, b[:]); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
