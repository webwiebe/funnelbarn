package funnelbarn

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The environment option was documented and silently dropped, so events
// arrived untagged and the dashboard filter had nothing to filter on. Every
// event the SDK sends must carry it.
func TestTrackAndPage_TagEnvironment(t *testing.T) {
	bodies := make(chan map[string]any, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		bodies <- b
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	Init(Options{APIKey: "k", Endpoint: srv.URL, ProjectName: "p", Environment: "staging"})
	Track("signup", nil)
	Page("https://example.com/", "")
	if err := Shutdown(3 * time.Second); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	for i := 0; i < 2; i++ {
		select {
		case b := <-bodies:
			if b["environment"] != "staging" {
				t.Errorf("event %d: environment = %v, want staging", i, b["environment"])
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for event %d", i)
		}
	}
}

func TestTrack_OmitsEnvironmentWhenUnset(t *testing.T) {
	bodies := make(chan map[string]any, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		bodies <- b
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	Init(Options{APIKey: "k", Endpoint: srv.URL})
	Track("signup", nil)
	_ = Shutdown(3 * time.Second)

	select {
	case b := <-bodies:
		if _, present := b["environment"]; present {
			t.Errorf("environment should be omitted when unset, got %v", b["environment"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out")
	}
}
