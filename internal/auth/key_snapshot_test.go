package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"
)

func keyHash(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// keyDB is a stored key table with a live lookup that counts its calls.
type keyDB struct {
	mu        sync.Mutex
	keys      map[string]KeyGrant
	listErr   error
	lookupErr error
	lookups   int
}

func (d *keyDB) list(context.Context) ([]KeyGrant, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.listErr != nil {
		return nil, d.listErr
	}
	out := make([]KeyGrant, 0, len(d.keys))
	for _, k := range d.keys {
		out = append(out, k)
	}
	return out, nil
}

func (d *keyDB) lookup(_ context.Context, sum string) (string, string, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lookups++
	if d.lookupErr != nil {
		return "", "", false, d.lookupErr
	}
	k, ok := d.keys[sum]
	return k.ProjectID, k.Scope, ok, nil
}

func (d *keyDB) add(plaintext, projectID, scope string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.keys[keyHash(plaintext)] = KeyGrant{KeySHA256: keyHash(plaintext), ProjectID: projectID, Scope: scope}
}

func (d *keyDB) revoke(plaintext string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.keys, keyHash(plaintext))
}

func (d *keyDB) lookupCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lookups
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func notAnalytics(scope string) bool { return scope != "analytics:read" }

func newSnapshotAuthorizer(t *testing.T) (*Authorizer, *KeySnapshot, *keyDB, *fakeClock) {
	t.Helper()
	db := &keyDB{keys: map[string]KeyGrant{}}
	clk := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	ks := NewKeySnapshot(db.list, notAnalytics, clk.Now)
	a := New("static-key").WithDBLookup(db.lookup, nil).WithEvaluateKeys(ks)
	return a, ks, db, clk
}

func TestValidForEvaluate_ServesSnapshotWithoutLookup(t *testing.T) {
	a, ks, db, _ := newSnapshotAuthorizer(t)
	db.add("sdk", "p1", "ingest")
	if err := ks.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	pid, scope, ok := a.ValidForEvaluate(context.Background(), "sdk")
	if !ok || pid != "p1" || scope != "ingest" {
		t.Fatalf("ValidForEvaluate = %q, %q, %v", pid, scope, ok)
	}
	if n := db.lookupCount(); n != 0 {
		t.Fatalf("a key in the snapshot must not hit the database, got %d lookups", n)
	}
	if _, scope, ok := a.ValidForEvaluate(context.Background(), "static-key"); !ok || scope != "full" {
		t.Fatalf("the static key must still work, got %q %v", scope, ok)
	}
}

// Before the first load the evaluate path does the live lookup.
func TestValidForEvaluate_ColdFallsBackToLookup(t *testing.T) {
	a, _, db, _ := newSnapshotAuthorizer(t)
	db.add("sdk", "p1", "ingest")
	if _, _, ok := a.ValidForEvaluate(context.Background(), "sdk"); !ok {
		t.Fatal("want the live lookup to accept the key")
	}
	if db.lookupCount() != 1 {
		t.Fatalf("want 1 live lookup, got %d", db.lookupCount())
	}
}

// A key created after the last load is not in the snapshot; the live lookup
// finds it. A failed or negative lookup is never remembered.
func TestValidForEvaluate_MissingKeyAndFailedLookupAreNotCached(t *testing.T) {
	a, ks, db, _ := newSnapshotAuthorizer(t)
	if err := ks.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	db.add("new", "p1", "ingest")
	if _, _, ok := a.ValidForEvaluate(context.Background(), "new"); !ok {
		t.Fatal("a key missing from the snapshot must fall back to the live lookup")
	}

	db.lookupErr = errors.New("database is locked")
	if _, _, ok := a.ValidForEvaluate(context.Background(), "new"); ok {
		t.Fatal("a failed lookup must refuse the key")
	}
	db.lookupErr = nil
	before := db.lookupCount()
	if _, _, ok := a.ValidForEvaluate(context.Background(), "new"); !ok {
		t.Fatal("the key must work again once the lookup succeeds")
	}
	if db.lookupCount() != before+1 {
		t.Fatal("the live lookup must run again: nothing from a lookup may be cached")
	}
	if _, _, ok := a.ValidForEvaluate(context.Background(), "unknown"); ok {
		t.Fatal("an unknown key must be refused")
	}
}

// analytics:read keys stay out of the snapshot and always use the live lookup.
func TestValidForEvaluate_ExcludedScopeUsesLookup(t *testing.T) {
	a, ks, db, _ := newSnapshotAuthorizer(t)
	db.add("reader", "p1", "analytics:read")
	if err := ks.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := a.ValidForEvaluate(context.Background(), "reader"); !ok {
		t.Fatal("want the live lookup to accept the key")
	}
	if db.lookupCount() != 1 {
		t.Fatalf("an excluded scope must use the live lookup, got %d lookups", db.lookupCount())
	}
}

// A revoked key leaves the snapshot on the next good refresh. While refreshes
// fail it keeps working until the snapshot is KeySnapshotMaxAge old, and after
// that the live lookup refuses it.
func TestValidForEvaluate_RevokeWindowAndMaxAge(t *testing.T) {
	a, ks, db, clk := newSnapshotAuthorizer(t)
	db.add("sdk", "p1", "full")
	db.add("other", "p1", "ingest")
	if err := ks.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Revoked, and the next refresh succeeds: refused at once.
	db.revoke("other")
	clk.advance(KeySnapshotInterval)
	if err := ks.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := a.ValidForEvaluate(context.Background(), "other"); ok {
		t.Fatal("a revoked key must be refused after the next good refresh")
	}

	// Revoked during a stall: refreshes fail, the snapshot still holds it.
	db.revoke("sdk")
	db.listErr = errors.New("disk stall")
	lookups := db.lookupCount()
	for _, at := range []time.Duration{KeySnapshotInterval, 30 * time.Second, KeySnapshotMaxAge} {
		clk.now = time.Unix(1_700_000_000, 0).Add(KeySnapshotInterval + at)
		_ = ks.Refresh(context.Background())
		if _, _, ok := a.ValidForEvaluate(context.Background(), "sdk"); !ok {
			t.Fatalf("at age %v the snapshot is within its cap and must answer", at)
		}
	}
	if db.lookupCount() != lookups {
		t.Fatal("within the cap the evaluate path must not touch the database")
	}

	// Past the cap: the live lookup decides, and the key is revoked.
	clk.advance(time.Second)
	if _, _, ok := a.ValidForEvaluate(context.Background(), "sdk"); ok {
		t.Fatal("past KeySnapshotMaxAge a revoked key must be refused by the live lookup")
	}
	if db.lookupCount() != lookups+1 {
		t.Fatal("past the cap the evaluate path must do the live lookup")
	}
	if age := ks.Age(); age <= KeySnapshotMaxAge {
		t.Fatalf("Age = %v, want past the cap", age)
	}
}

// The snapshot only answers ValidForEvaluate. ValidWithProject, which every
// other route uses, checks a revoked key live at once.
func TestValidWithProject_IgnoresSnapshot(t *testing.T) {
	a, ks, db, _ := newSnapshotAuthorizer(t)
	db.add("sdk", "p1", "full")
	if err := ks.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	db.revoke("sdk")
	if _, _, ok := a.ValidForEvaluate(context.Background(), "sdk"); !ok {
		t.Fatal("the evaluate path may still accept it within the window")
	}
	if _, _, ok := a.ValidWithProject(context.Background(), "sdk"); ok {
		t.Fatal("every other route must refuse a revoked key at once")
	}
}

// Without a snapshot, ValidForEvaluate is ValidWithProject.
func TestValidForEvaluate_NoSnapshot(t *testing.T) {
	db := &keyDB{keys: map[string]KeyGrant{}}
	db.add("sdk", "p1", "ingest")
	a := New("").WithDBLookup(db.lookup, nil)
	if pid, _, ok := a.ValidForEvaluate(context.Background(), "sdk"); !ok || pid != "p1" {
		t.Fatalf("got %q %v", pid, ok)
	}
	var nilAuth *Authorizer
	if _, scope, ok := nilAuth.ValidForEvaluate(context.Background(), "x"); !ok || scope != "full" {
		t.Fatal("a nil authorizer allows everything, as ValidWithProject does")
	}
	if b := nilAuth.WithEvaluateKeys(nil); b == nil {
		t.Fatal("WithEvaluateKeys on nil must return an Authorizer")
	}
}

// On a snapshot hit the last_used_at touch runs off the request: a touch that
// blocks, as a SQLite write does in a stall, does not hold ValidForEvaluate.
// It runs at most once per key per snapshotTouchInterval.
func TestValidForEvaluate_TouchDoesNotBlockAndIsDebounced(t *testing.T) {
	db := &keyDB{keys: map[string]KeyGrant{}}
	db.add("sdk", "p1", "ingest")
	clk := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	ks := NewKeySnapshot(db.list, notAnalytics, clk.Now)
	release := make(chan struct{})
	defer close(release)
	touches := make(chan string, 10)
	touch := func(ctx context.Context, sum string) error {
		touches <- sum
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	a := New("").WithDBLookup(db.lookup, touch).WithEvaluateKeys(ks)
	if err := ks.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		done := make(chan bool, 1)
		go func() {
			_, _, ok := a.ValidForEvaluate(context.Background(), "sdk")
			done <- ok
		}()
		select {
		case ok := <-done:
			if !ok {
				t.Fatal("want the key accepted")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("ValidForEvaluate blocked on the touch")
		}
	}
	select {
	case got := <-touches:
		if got != keyHash("sdk") {
			t.Fatalf("touched %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the touch never ran")
	}
	select {
	case <-touches:
		t.Fatal("a second touch within the interval must be skipped")
	case <-time.After(50 * time.Millisecond):
	}

	clk.advance(snapshotTouchInterval)
	if _, _, ok := a.ValidForEvaluate(context.Background(), "sdk"); !ok {
		t.Fatal("want the key accepted")
	}
	select {
	case <-touches:
	case <-time.After(5 * time.Second):
		t.Fatal("after the interval the key must be touched again")
	}
}
