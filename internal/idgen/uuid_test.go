package idgen

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestUUID_IsVersion4(t *testing.T) {
	id, err := UUID()
	if err != nil {
		t.Fatal(err)
	}
	if !uuidV4.MatchString(id) {
		t.Fatalf("%q is not a v4 UUID", id)
	}
}

func TestUUIDFrom_SetsVersionAndVariantBits(t *testing.T) {
	id, err := UUIDFrom(strings.NewReader(strings.Repeat("\xff", 16)))
	if err != nil {
		t.Fatal(err)
	}
	if id != "ffffffff-ffff-4fff-bfff-ffffffffffff" {
		t.Fatalf("got %q", id)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

func TestUUIDFrom_ReturnsReadError(t *testing.T) {
	if _, err := UUIDFrom(failingReader{}); err == nil || !strings.Contains(err.Error(), "no entropy") {
		t.Fatalf("want the read error, got %v", err)
	}
}
