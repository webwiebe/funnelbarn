package spool

import "os"

// SetFileOps replaces the rename and reopen Rotate uses, so tests can make
// either one fail.
func SetFileOps(s *Spool, rename func(oldpath, newpath string) error, open func(path string) (*os.File, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rename = rename
	s.open = open
}

// OpenActive is the reopen New and Rotate use by default.
var OpenActive = openActive
