package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Authorizer — New / NewHashed
// ---------------------------------------------------------------------------

func TestNew_ValidKey(t *testing.T) {
	a := New("mysecretkey")
	if !a.Enabled() {
		t.Fatal("expected Enabled() == true")
	}
}

func TestNew_EmptyKey(t *testing.T) {
	a := New("")
	if a.Enabled() {
		t.Fatal("expected Enabled() == false for empty key")
	}
}

func TestNewHashed_ValidHex(t *testing.T) {
	sum := sha256.Sum256([]byte("plaintext"))
	hexSum := hex.EncodeToString(sum[:])
	a, err := NewHashed(hexSum)
	if err != nil {
		t.Fatalf("NewHashed: unexpected error: %v", err)
	}
	if !a.Enabled() {
		t.Fatal("expected Enabled() == true")
	}
}

func TestNewHashed_InvalidHex(t *testing.T) {
	_, err := NewHashed("notvalidhex!!")
	if err == nil {
		t.Fatal("expected error for invalid hex")
	}
}

func TestNewHashed_WrongLength(t *testing.T) {
	_, err := NewHashed("deadbeef") // too short
	if err == nil {
		t.Fatal("expected error for wrong length hex")
	}
}

func TestNewHashed_Empty(t *testing.T) {
	a, err := NewHashed("")
	if err != nil {
		t.Fatalf("NewHashed empty: unexpected error: %v", err)
	}
	if a.Enabled() {
		t.Fatal("expected Enabled() == false for empty hash")
	}
}

// ---------------------------------------------------------------------------
// Authorizer.ValidWithProject — static key
// ---------------------------------------------------------------------------

func TestAuthorizer_ValidWithProject_StaticKey(t *testing.T) {
	a := New("correct-key")

	_, _, ok := a.ValidWithProject(context.Background(), "correct-key")
	if !ok {
		t.Fatal("expected valid for correct key")
	}

	_, _, ok = a.ValidWithProject(context.Background(), "wrong-key")
	if ok {
		t.Fatal("expected invalid for wrong key")
	}
}

func TestAuthorizer_ValidWithProject_Disabled(t *testing.T) {
	a := New("")
	_, scope, ok := a.ValidWithProject(context.Background(), "anything")
	if !ok {
		t.Fatal("expected ok=true when auth is disabled")
	}
	if scope != "full" {
		t.Errorf("expected scope=full, got %q", scope)
	}
}

// ---------------------------------------------------------------------------
// Authorizer.ValidWithProject — DB lookup
// ---------------------------------------------------------------------------

func TestAuthorizer_ValidWithProject_DBLookup(t *testing.T) {
	const projectID = "proj-1"
	const testKey = "db-key-abc"

	sum := sha256.Sum256([]byte(testKey))
	hexSum := hex.EncodeToString(sum[:])

	lookup := func(_ context.Context, keySHA256 string) (string, string, bool, error) {
		if keySHA256 == hexSum {
			return projectID, "ingest", true, nil
		}
		return "", "", false, nil
	}

	touched := false
	touch := func(_ context.Context, _ string) error {
		touched = true
		return nil
	}

	a := New("").WithDBLookup(lookup, touch)

	pid, scope, ok := a.ValidWithProject(context.Background(), testKey)
	if !ok {
		t.Fatal("expected ok=true for valid DB key")
	}
	if pid != projectID {
		t.Errorf("expected projectID=%q, got %q", projectID, pid)
	}
	if scope != "ingest" {
		t.Errorf("expected scope=ingest, got %q", scope)
	}
	if !touched {
		t.Error("expected touch callback to be called")
	}
}

func TestAuthorizer_ValidWithProject_DBLookup_NotFound(t *testing.T) {
	lookup := func(_ context.Context, _ string) (string, string, bool, error) {
		return "", "", false, nil
	}
	a := New("").WithDBLookup(lookup, nil)

	_, _, ok := a.ValidWithProject(context.Background(), "unknown-key")
	if ok {
		t.Fatal("expected ok=false for unknown DB key")
	}
}

// ---------------------------------------------------------------------------
// UserAuthenticator
// ---------------------------------------------------------------------------

func TestUserAuthenticator_Valid(t *testing.T) {
	a, err := NewUserAuthenticator("admin", "secret", "")
	if err != nil {
		t.Fatalf("NewUserAuthenticator: %v", err)
	}
	if !a.Enabled() {
		t.Fatal("expected Enabled()")
	}

	if !a.Valid("admin", "secret") {
		t.Error("expected Valid for correct credentials")
	}
	if a.Valid("admin", "wrong") {
		t.Error("expected Invalid for wrong password")
	}
	if a.Valid("other", "secret") {
		t.Error("expected Invalid for wrong username")
	}
}

func TestUserAuthenticator_BcryptPreHashed(t *testing.T) {
	// Use a pre-computed bcrypt hash for "password123".
	// bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	// We generate it here to keep the test self-contained.
	a, err := NewUserAuthenticator("user", "password123", "")
	if err != nil {
		t.Fatalf("create auth: %v", err)
	}

	// Extract the hash and create a new authenticator from it (simulating startup from env).
	hash := string(a.passwordHash)
	a2, err := NewUserAuthenticator("user", "", hash)
	if err != nil {
		t.Fatalf("NewUserAuthenticator from bcrypt: %v", err)
	}
	if !a2.Valid("user", "password123") {
		t.Error("expected Valid from pre-hashed bcrypt")
	}
}

func TestUserAuthenticator_Empty(t *testing.T) {
	a, err := NewUserAuthenticator("", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Enabled() {
		t.Fatal("expected Enabled()==false for empty credentials")
	}
	// When disabled, Valid should return true (allow through).
	if !a.Valid("anything", "anything") {
		t.Error("expected Valid==true when auth disabled")
	}
}

func TestUserAuthenticator_Username(t *testing.T) {
	a, _ := NewUserAuthenticator("bob", "pw", "")
	if a.Username() != "bob" {
		t.Errorf("Username() = %q, want %q", a.Username(), "bob")
	}
	var nilAuth *UserAuthenticator
	if nilAuth.Username() != "" {
		t.Error("nil Username() should return empty string")
	}
}

// ---------------------------------------------------------------------------
// SessionManager
// ---------------------------------------------------------------------------

func TestSessionManager_CreateOpaqueToken(t *testing.T) {
	sm := NewSessionManager("test-secret-key", time.Hour)

	token, expires, err := sm.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	if expires.IsZero() {
		t.Fatal("expected non-zero expiry")
	}
	if got := time.Until(expires); got < 55*time.Minute || got > 65*time.Minute {
		t.Errorf("expiry should be ~1h out, got %v", got)
	}
	// Opaque handles must be unique and carry no structure the old
	// payload.signature scheme had.
	other, _, _ := sm.Create()
	if token == other {
		t.Error("two session tokens must never collide")
	}
	if strings.Contains(token, ".") {
		t.Errorf("opaque handle must not look like a signed token: %q", token)
	}
}

func TestSessionManager_DefaultTTL(t *testing.T) {
	sm := NewSessionManager("secret", 0)
	if sm.TTL() != 12*time.Hour {
		t.Errorf("default TTL: want 12h, got %v", sm.TTL())
	}
	var nilSM *SessionManager
	if nilSM.TTL() != 0 {
		t.Error("nil SessionManager.TTL should be 0")
	}
}

func TestSessionManager_NilManager(t *testing.T) {
	var sm *SessionManager
	if _, _, err := sm.Create(); err == nil {
		t.Error("nil SessionManager.Create should return error")
	}
	if sm.CSRFToken("tok") != "" {
		t.Error("nil SessionManager.CSRFToken should be empty")
	}
}

func TestHashSessionToken(t *testing.T) {
	h1 := HashSessionToken("token-a")
	h2 := HashSessionToken("token-a")
	if h1 != h2 {
		t.Error("HashSessionToken not deterministic")
	}
	if len(h1) != 64 {
		t.Errorf("expected 64 hex chars (sha256), got %d", len(h1))
	}
	if HashSessionToken("token-b") == h1 {
		t.Error("different tokens must hash differently")
	}
	// Whitespace-trimmed so a cookie value with stray spaces still matches.
	if HashSessionToken(" token-a ") != h1 {
		t.Error("HashSessionToken must trim whitespace")
	}
}

// ---------------------------------------------------------------------------
// Cookies
// ---------------------------------------------------------------------------

func TestSessionCookie(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	c := SessionCookie("my-token", expires, true)
	if c.Name != "funnelbarn_session" {
		t.Errorf("cookie name: got %q", c.Name)
	}
	if c.Value != "my-token" {
		t.Errorf("cookie value: got %q", c.Value)
	}
	if !c.HttpOnly {
		t.Error("session cookie must be HttpOnly")
	}
	if !c.Secure {
		t.Error("expected Secure=true")
	}
}

func TestClearSessionCookie(t *testing.T) {
	c := ClearSessionCookie(false)
	if c.MaxAge != -1 {
		t.Errorf("expected MaxAge=-1, got %d", c.MaxAge)
	}
	if c.Value != "" {
		t.Errorf("expected empty value, got %q", c.Value)
	}
}

func TestCSRFToken_Deterministic(t *testing.T) {
	sm := NewSessionManager("csrf-secret", time.Hour)
	t1 := sm.CSRFToken("session-token")
	t2 := sm.CSRFToken("session-token")
	if t1 != t2 {
		t.Error("CSRFToken not deterministic")
	}
	if len(t1) != 32 {
		t.Errorf("expected 32 hex chars, got %d: %q", len(t1), t1)
	}
}

func TestCSRFToken_DifferentInputs(t *testing.T) {
	sm := NewSessionManager("csrf-secret", time.Hour)
	t1 := sm.CSRFToken("token-a")
	t2 := sm.CSRFToken("token-b")
	if t1 == t2 {
		t.Error("different session tokens should yield different CSRF tokens")
	}
}

// TestCSRFToken_KeyedBySessionSecret pins the fix for the hard-coded "csrf"
// HMAC key: the token must depend on the session secret, so an attacker who
// knows the algorithm but not the secret cannot compute it.
func TestCSRFToken_KeyedBySessionSecret(t *testing.T) {
	smA := NewSessionManager("secret-a-is-long-enough-000000000", time.Hour)
	smB := NewSessionManager("secret-b-is-long-enough-000000000", time.Hour)
	if smA.CSRFToken("same-session-token") == smB.CSRFToken("same-session-token") {
		t.Error("CSRF token must be keyed by the session secret, not a constant")
	}
}

func TestCSRFCookie(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	c := CSRFCookie("csrf-value", expires, false)
	if c.HttpOnly {
		t.Error("CSRF cookie must NOT be HttpOnly (needs JS access)")
	}
	if c.Name != "funnelbarn_csrf" {
		t.Errorf("cookie name: got %q", c.Name)
	}
	if c.Value != "csrf-value" {
		t.Errorf("cookie value: got %q", c.Value)
	}
}

func TestClearCSRFCookie(t *testing.T) {
	c := ClearCSRFCookie(true)
	if c.MaxAge != -1 {
		t.Errorf("expected MaxAge=-1, got %d", c.MaxAge)
	}
}

// ---------------------------------------------------------------------------
// HeaderAPIKey constant
// ---------------------------------------------------------------------------

func TestHeaderAPIKey(t *testing.T) {
	if strings.ToLower(HeaderAPIKey) != HeaderAPIKey {
		t.Error("HTTP header should be lowercase")
	}
}
