package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
)

func chunkRequest(t *testing.T, key string, body map[string]any, header string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatalf("encode: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recordings/chunk", &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderAPIKey, key)
	if header != "" {
		req.Header.Set("x-funnelbarn-environment", header)
	}
	return req
}

// A recording is filed under an environment the way an event is: what the SDK
// sent, canonicalised, and production when it sent nothing (#560).
func TestIngestRecordingChunk_FilesTheEnvironment(t *testing.T) {
	srv, store := newRecordingServer(t)
	_, key := projectKey(t, store, "Rec Env", "rec-env")

	cases := []struct{ id, sent, header, want string }{
		{"rec-env-test", "test", "", "test"},
		{"rec-env-alias", "prod", "", "production"},
		{"rec-env-header", "", "staging", "staging"},
		{"rec-env-none", "", "", "production"},
		{"rec-env-typo", "tset", "", "development"},
	}
	for _, c := range cases {
		body := validChunk(c.id)
		if c.sent != "" {
			body["environment"] = c.sent
		}
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, chunkRequest(t, key, body, c.header))
		if w.Code != http.StatusAccepted {
			t.Fatalf("%s: got %d: %s", c.id, w.Code, w.Body.String())
		}
		rec, err := store.GetRecording(context.Background(), c.id)
		if err != nil {
			t.Fatalf("%s: %v", c.id, err)
		}
		if rec.Environment != c.want {
			t.Errorf("%s: environment %q, want %q", c.id, rec.Environment, c.want)
		}
	}
}

// A key mapped to an environment overrides what the SDK sent, as for events.
func TestIngestRecordingChunk_KeyEnvironmentOverrides(t *testing.T) {
	srv, store := newRecordingServer(t)
	projectID, _ := projectKey(t, store, "Rec Key Env", "rec-key-env")
	plain := "mapped-key"
	sum := sha256.Sum256([]byte(plain))
	k, err := store.CreateAPIKey(context.Background(), "mapped", projectID, hex.EncodeToString(sum[:]), "ingest")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if err := store.SetAPIKeyEnvironment(context.Background(), k.ID, "staging"); err != nil {
		t.Fatalf("SetAPIKeyEnvironment: %v", err)
	}
	body := validChunk("rec-key-env-1")
	body["environment"] = "production"
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, chunkRequest(t, plain, body, ""))
	if w.Code != http.StatusAccepted {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	rec, err := store.GetRecording(context.Background(), "rec-key-env-1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Environment != "staging" {
		t.Errorf("environment %q, want staging", rec.Environment)
	}
}
