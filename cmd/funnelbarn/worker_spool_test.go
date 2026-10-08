package main

import (
	"errors"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/workerhealth"
)

func TestSkipMalformedAdvancesCursor(t *testing.T) {
	dir := t.TempDir()
	entry := spool.RecordAtOffset{EndOffset: 1469, Malformed: errors.New("bad json")}

	if got := skipMalformed(dir, entry, 1000); got != 1469 {
		t.Fatalf("offset = %d, want 1469", got)
	}
	cursor, err := spool.ReadCursor(dir)
	if err != nil {
		t.Fatalf("ReadCursor: %v", err)
	}
	if cursor != 1469 {
		t.Errorf("cursor = %d, want 1469", cursor)
	}
}

func TestCheckSpoolProgressWithoutSpool(t *testing.T) {
	health := workerhealth.New(workerhealth.Options{})
	checkSpoolProgress(health, t.TempDir(), 0)
	if health.Stalled() {
		t.Error("an empty spool directory must not count as stalled")
	}
}
