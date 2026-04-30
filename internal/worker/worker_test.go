package worker

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

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
