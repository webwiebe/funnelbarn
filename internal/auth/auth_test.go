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

func TestSessionManager_CreateAndValidate(t *testing.T) {
	sm := NewSessionManager("test-secret-key", time.Hour)

	token, expires, err := sm.Create("alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	if expires.IsZero() {
		t.Fatal("expected non-zero expiry")
	}

	username, ok := sm.Valid(token)
	if !ok {
		t.Fatal("expected Valid==true for fresh token")
	}
	if username != "alice" {
		t.Errorf("username: want %q, got %q", "alice", username)
	}
}

func TestSessionManager_ExpiredToken(t *testing.T) {
	sm := NewSessionManager("secret", time.Hour)
	// Override `now` to be in the future relative to token creation.
	pastSM := &SessionManager{
		secret: []byte("secret"),
		ttl:    -time.Second, // negative TTL → already expired
		now:    time.Now,
	}

	token, _, err := pastSM.Create("alice")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, ok := sm.Valid(token)
	if ok {
		t.Error("expected expired token to be invalid")
	}
}

func TestSessionManager_WrongSecret(t *testing.T) {
	sm1 := NewSessionManager("secret-a", time.Hour)
	sm2 := NewSessionManager("secret-b", time.Hour)

	token, _, _ := sm1.Create("alice")
	_, ok := sm2.Valid(token)
	if ok {
		t.Error("token signed with different secret should be invalid")
	}
}

func TestSessionManager_TamperedToken(t *testing.T) {
	sm := NewSessionManager("secret", time.Hour)
	token, _, _ := sm.Create("alice")

	// Tamper with the token by appending a character.
	tampered := token + "x"
	_, ok := sm.Valid(tampered)
	if ok {
		t.Error("tampered token should be invalid")
	}
}

func TestSessionManager_EmptyToken(t *testing.T) {
	sm := NewSessionManager("secret", time.Hour)
	_, ok := sm.Valid("")
	if ok {
		t.Error("empty token should be invalid")
	}
}

func TestSessionManager_NilManager(t *testing.T) {
	var sm *SessionManager
	_, ok := sm.Valid("anything")
	if ok {
		t.Error("nil SessionManager.Valid should return false")
	}
	_, _, err := sm.Create("alice")
	if err == nil {
		t.Error("nil SessionManager.Create should return error")
	}
}

func TestSessionManager_RandomSecretWhenEmpty(t *testing.T) {
	sm := NewSessionManager("", time.Hour)
	token, _, err := sm.Create("bob")
	if err != nil {
		t.Fatalf("Create with random secret: %v", err)
	}
	username, ok := sm.Valid(token)
	if !ok || username != "bob" {
		t.Error("token from randomly-seeded manager should be valid within same instance")
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
	t1 := CSRFToken("session-token")
	t2 := CSRFToken("session-token")
	if t1 != t2 {
		t.Error("CSRFToken not deterministic")
	}
	if len(t1) != 32 {
		t.Errorf("expected 32 hex chars, got %d: %q", len(t1), t1)
	}
}

func TestCSRFToken_DifferentInputs(t *testing.T) {
	t1 := CSRFToken("token-a")
	t2 := CSRFToken("token-b")
	if t1 == t2 {
		t.Error("different session tokens should yield different CSRF tokens")
	}
}

func TestCSRFCookie(t *testing.T) {
	expires := time.Now().Add(time.Hour)
	c := CSRFCookie("tok", expires, false)
	if c.HttpOnly {
		t.Error("CSRF cookie must NOT be HttpOnly (needs JS access)")
	}
	if c.Name != "funnelbarn_csrf" {
		t.Errorf("cookie name: got %q", c.Name)
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
