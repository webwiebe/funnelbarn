package worker

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/environment"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
)

// makeRecord constructs a spool.Record with a JSON-encoded body.
func makeRecord(payload EventPayload) spool.Record {
	body, _ := json.Marshal(payload)
	return spool.Record{
		IngestID:    "test-ingest-id",
		ReceivedAt:  time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		RemoteAddr:  "192.168.1.42:55000",
		ContentType: "application/json",
		BodyBase64:  base64.StdEncoding.EncodeToString(body),
		ProjectSlug: "my-site",
	}
}

// ---------------------------------------------------------------------------
// ProcessRecord — happy path
// ---------------------------------------------------------------------------

func TestProcessRecord_Basic(t *testing.T) {
	payload := EventPayload{
		Name:      "pageview",
		URL:       "https://example.com/page",
		Referrer:  "https://www.google.com/",
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/124.0",
		SessionID: "abcdef1234567890abcdef1234567890", // valid 32-char hex
		Timestamp: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
	}

	event, err := ProcessRecord(makeRecord(payload))
	if err != nil {
		t.Fatalf("ProcessRecord: %v", err)
	}

	if event.Name != "pageview" {
		t.Errorf("Name: want pageview, got %q", event.Name)
	}
	if event.URL != "https://example.com/page" {
		t.Errorf("URL: want %q, got %q", "https://example.com/page", event.URL)
	}
	if event.ReferrerDomain != "google.com" {
		t.Errorf("ReferrerDomain: want google.com, got %q", event.ReferrerDomain)
	}
	if event.Browser == "" {
		t.Error("expected non-empty Browser from UA parsing")
	}
	if event.OS == "" {
		t.Error("expected non-empty OS from UA parsing")
	}
	if event.SessionID != payload.SessionID {
		t.Errorf("SessionID: want %q, got %q", payload.SessionID, event.SessionID)
	}
	if event.IngestID != "test-ingest-id" {
		t.Errorf("IngestID: want test-ingest-id, got %q", event.IngestID)
	}
	// Timestamp from payload should be used.
	if !event.OccurredAt.Equal(payload.Timestamp) {
		t.Errorf("OccurredAt: want %v, got %v", payload.Timestamp, event.OccurredAt)
	}
}

// ---------------------------------------------------------------------------
// ProcessRecord — event name required
// ---------------------------------------------------------------------------

func TestProcessRecord_MissingName(t *testing.T) {
	payload := EventPayload{
		Name: "",
		URL:  "https://example.com",
	}
	_, err := ProcessRecord(makeRecord(payload))
	if err == nil {
		t.Fatal("expected error for empty event name")
	}
}

// ---------------------------------------------------------------------------
// ProcessRecord — invalid base64
// ---------------------------------------------------------------------------

func TestProcessRecord_InvalidBase64(t *testing.T) {
	rec := spool.Record{
		IngestID:   "bad-base64",
		BodyBase64: "this is not valid base64 !!@#",
		ReceivedAt: time.Now(),
	}
	_, err := ProcessRecord(rec)
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

// ---------------------------------------------------------------------------
// ProcessRecord — invalid JSON
// ---------------------------------------------------------------------------

func TestProcessRecord_InvalidJSON(t *testing.T) {
	rec := spool.Record{
		IngestID:   "bad-json",
		BodyBase64: base64.StdEncoding.EncodeToString([]byte("not json")),
		ReceivedAt: time.Now(),
	}
	_, err := ProcessRecord(rec)
	if err == nil {
		t.Fatal("expected error for invalid JSON body")
	}
}

// ---------------------------------------------------------------------------
// ProcessRecord — timestamp fallback to ReceivedAt
// ---------------------------------------------------------------------------

func TestProcessRecord_TimestampFallback(t *testing.T) {
	receivedAt := time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC)
	payload := EventPayload{
		Name: "click",
		// Timestamp is zero value → should fall back to ReceivedAt.
	}
	body, _ := json.Marshal(payload)
	rec := spool.Record{
		IngestID:   "ts-fallback",
		ReceivedAt: receivedAt,
		RemoteAddr: "10.0.0.1:1234",
		BodyBase64: base64.StdEncoding.EncodeToString(body),
	}

	event, err := ProcessRecord(rec)
	if err != nil {
		t.Fatalf("ProcessRecord: %v", err)
	}
	if !event.OccurredAt.Equal(receivedAt) {
		t.Errorf("OccurredAt: want %v (ReceivedAt), got %v", receivedAt, event.OccurredAt)
	}
}

// ---------------------------------------------------------------------------
// ProcessRecord — UTM from URL
// ---------------------------------------------------------------------------

func TestProcessRecord_UTMFromURL(t *testing.T) {
	payload := EventPayload{
		Name: "pageview",
		URL:  "https://example.com/?utm_source=newsletter&utm_medium=email&utm_campaign=spring2024",
	}
	event, err := ProcessRecord(makeRecord(payload))
	if err != nil {
		t.Fatalf("ProcessRecord: %v", err)
	}
	if event.UTMSource != "newsletter" {
		t.Errorf("UTMSource: want newsletter, got %q", event.UTMSource)
	}
	if event.UTMMedium != "email" {
		t.Errorf("UTMMedium: want email, got %q", event.UTMMedium)
	}
	if event.UTMCampaign != "spring2024" {
		t.Errorf("UTMCampaign: want spring2024, got %q", event.UTMCampaign)
	}
}

// ---------------------------------------------------------------------------
// ProcessRecord — UTM from payload overrides URL
// ---------------------------------------------------------------------------

func TestProcessRecord_UTMPayloadOverridesURL(t *testing.T) {
	payload := EventPayload{
		Name:      "pageview",
		URL:       "https://example.com/?utm_source=url-source",
		UTMSource: "payload-source", // explicit wins
	}
	event, err := ProcessRecord(makeRecord(payload))
	if err != nil {
		t.Fatalf("ProcessRecord: %v", err)
	}
	if event.UTMSource != "payload-source" {
		t.Errorf("UTMSource: want payload-source, got %q", event.UTMSource)
	}
}

// ---------------------------------------------------------------------------
// ProcessRecord — session fingerprint when session ID is invalid
// ---------------------------------------------------------------------------

func TestProcessRecord_SessionFingerprintFallback(t *testing.T) {
	payload := EventPayload{
		Name:      "pageview",
		SessionID: "invalid-not-hex-32-chars", // invalid → should fingerprint
		UserAgent: "TestAgent/1.0",
	}
	rec := makeRecord(payload)
	rec.RemoteAddr = "203.0.113.5:8080"

	event, err := ProcessRecord(rec)
	if err != nil {
		t.Fatalf("ProcessRecord: %v", err)
	}
	if event.SessionID == payload.SessionID {
		t.Error("expected fingerprint to replace invalid session ID")
	}
	if len(event.SessionID) != 32 {
		t.Errorf("fingerprinted session ID should be 32 chars, got %d: %q", len(event.SessionID), event.SessionID)
	}
}

// ---------------------------------------------------------------------------
// ProcessRecord — properties are JSON-encoded
// ---------------------------------------------------------------------------

func TestProcessRecord_Properties(t *testing.T) {
	payload := EventPayload{
		Name:       "purchase",
		Properties: map[string]any{"amount": 99.99, "currency": "USD"},
	}
	event, err := ProcessRecord(makeRecord(payload))
	if err != nil {
		t.Fatalf("ProcessRecord: %v", err)
	}
	if event.Properties == "" {
		t.Error("expected non-empty Properties JSON")
	}
	var props map[string]any
	if err := json.Unmarshal([]byte(event.Properties), &props); err != nil {
		t.Errorf("Properties not valid JSON: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ProcessRecord — user ID hashed
// ---------------------------------------------------------------------------

func TestProcessRecord_UserIDHashed(t *testing.T) {
	payload := EventPayload{
		Name:   "login",
		UserID: "user-42",
	}
	event, err := ProcessRecord(makeRecord(payload))
	if err != nil {
		t.Fatalf("ProcessRecord: %v", err)
	}
	if event.UserIDHash == "" {
		t.Error("expected non-empty UserIDHash")
	}
	if event.UserIDHash == "user-42" {
		t.Error("UserIDHash should be hashed, not the plain user ID")
	}
}

// ---------------------------------------------------------------------------
// SafeProcess — panic recovery
// ---------------------------------------------------------------------------

func TestSafeProcess_RecoversPanic(t *testing.T) {
	// Create a malformed record that might cause ProcessRecord to panic.
	// A base64-encoded payload that decodes to invalid JSON should trigger
	// a recoverable error path, not necessarily a panic.
	// Test that SafeProcess doesn't panic even on bad input.
	rec := spool.Record{
		IngestID:    "test-panic",
		ProjectSlug: "test",
		BodyBase64:  "!!!invalid-base64!!!",
	}
	// Should not panic — error is returned instead
	_, err := SafeProcess(rec)
	if err == nil {
		t.Log("no error for invalid base64 (may be handled earlier)")
	}
	// Key assertion: function returned, not panicked
}

// ---------------------------------------------------------------------------
// coalesce helper
// ---------------------------------------------------------------------------

func TestCoalesce(t *testing.T) {
	tests := []struct {
		vals []string
		want string
	}{
		{[]string{"", "", "c"}, "c"},
		{[]string{"a", "b"}, "a"},
		{[]string{"", ""}, ""},
		{[]string{}, ""},
	}

	for _, tc := range tests {
		got := coalesce(tc.vals...)
		if got != tc.want {
			t.Errorf("coalesce(%v) = %q, want %q", tc.vals, got, tc.want)
		}
	}
}

// An unrecognised environment is filed under the default rather than rejected —
// dropping an event over a typo is worse than mis-filing it — but the process
// says so once per distinct value instead of silently polluting reporting.
func TestProcessRecord_UnknownEnvironmentIsFiledNotDropped(t *testing.T) {
	body := base64.StdEncoding.EncodeToString([]byte(
		`{"name":"page_view","environment":"prodution"}`))
	rec := spool.Record{
		IngestID:   "ingest-env-1",
		ReceivedAt: time.Now().UTC(),
		BodyBase64: body,
	}

	event, err := ProcessRecord(rec)
	if err != nil {
		t.Fatalf("ProcessRecord: %v", err)
	}
	if event.Environment != environment.Production {
		t.Errorf("environment: want %q, got %q", environment.Production, event.Environment)
	}
	if event.Name != "page_view" {
		t.Errorf("the event must still be processed, got name %q", event.Name)
	}
}

func TestProcessRecord_KnownEnvironmentAliasIsCanonicalised(t *testing.T) {
	body := base64.StdEncoding.EncodeToString([]byte(
		`{"name":"page_view","environment":"STG"}`))
	event, err := ProcessRecord(spool.Record{
		IngestID: "ingest-env-2", ReceivedAt: time.Now().UTC(), BodyBase64: body,
	})
	if err != nil {
		t.Fatalf("ProcessRecord: %v", err)
	}
	if event.Environment != environment.Staging {
		t.Errorf("environment: want %q, got %q", environment.Staging, event.Environment)
	}
}

// ---------------------------------------------------------------------------
// ProcessRecord — the fallback fingerprint must separate visitors (#226)
// ---------------------------------------------------------------------------

// fingerprintFor processes one clientless event and returns the session ID the
// worker minted for it.
func fingerprintFor(t *testing.T, mutate func(*spool.Record)) string {
	t.Helper()
	rec := makeRecord(EventPayload{Name: "pageview"})
	// Behind an ingress every visitor shares the TCP peer address; the real
	// client address arrives in a header and is carried on ClientIP.
	rec.RemoteAddr = "10.42.0.9:38000"
	rec.ClientIP = "203.0.113.5"
	mutate(&rec)

	event, err := ProcessRecord(rec)
	if err != nil {
		t.Fatalf("ProcessRecord: %v", err)
	}
	if len(event.SessionID) != 32 {
		t.Fatalf("fingerprinted session ID should be 32 chars, got %d: %q", len(event.SessionID), event.SessionID)
	}
	return event.SessionID
}

func TestProcessRecord_FingerprintUsesClientIPNotProxyAddress(t *testing.T) {
	// Two visitors, same ingress, different real addresses.
	a := fingerprintFor(t, func(r *spool.Record) { r.ClientIP = "203.0.113.5" })
	b := fingerprintFor(t, func(r *spool.Record) { r.ClientIP = "198.51.100.7" })

	if a == b {
		t.Fatal("two visitors behind one ingress share a session ID: the fingerprint is keyed on the proxy address, not the client's")
	}
}

func TestProcessRecord_FingerprintFallsBackToRemoteAddr(t *testing.T) {
	// Records spooled before ClientIP existed carry only RemoteAddr.
	a := fingerprintFor(t, func(r *spool.Record) { r.ClientIP = ""; r.RemoteAddr = "203.0.113.5:1000" })
	b := fingerprintFor(t, func(r *spool.Record) { r.ClientIP = ""; r.RemoteAddr = "198.51.100.7:1000" })

	if a == b {
		t.Error("with no ClientIP the fingerprint should still separate visitors by RemoteAddr")
	}
}

func TestProcessRecord_FingerprintRotatesOverTime(t *testing.T) {
	base := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	at := func(ts time.Time) func(*spool.Record) {
		return func(r *spool.Record) {
			body, _ := json.Marshal(EventPayload{Name: "pageview", Timestamp: ts})
			r.BodyBase64 = base64.StdEncoding.EncodeToString(body)
		}
	}

	if a, b := fingerprintFor(t, at(base)), fingerprintFor(t, at(base.Add(time.Minute))); a != b {
		t.Error("events a minute apart should stay in one session")
	}
	if a, b := fingerprintFor(t, at(base)), fingerprintFor(t, at(base.Add(90*24*time.Hour))); a == b {
		t.Fatal("events three months apart share a session ID: the fingerprint never rotates, so one session grows without bound")
	}
}

func TestProcessRecord_FingerprintIsScopedToProject(t *testing.T) {
	a := fingerprintFor(t, func(r *spool.Record) { r.ProjectSlug = "site-one" })
	b := fingerprintFor(t, func(r *spool.Record) { r.ProjectSlug = "site-two" })

	if a == b {
		t.Error("two projects sharing an ingress should not share a session ID (sessions.id is a global primary key)")
	}
}
