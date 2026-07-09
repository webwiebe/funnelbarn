package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func storeInstanceSettings(cfg *ServerConfig) InstanceSettingsRepo {
	return cfg.DB.(InstanceSettingsRepo)
}

func TestHandleGetInstanceSettings_NilRepo(t *testing.T) {
	// newTestServer wires no instance settings repo -> 200 with empty map.
	srv, _ := newTestServer(t)
	w := getJSON(t, srv, "/api/v1/instance-settings", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandlePutInstanceSettings_NilRepo(t *testing.T) {
	srv, _ := newTestServer(t)
	w := putJSON(t, srv, "/api/v1/instance-settings", map[string]string{"k": "v"}, nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleInstanceSettings_RoundTrip(t *testing.T) {
	srv, _ := fullServer(t, func(cfg *ServerConfig) { cfg.InstanceSettings = storeInstanceSettings(cfg) })

	// PUT sets values and returns the full settings map.
	wp := putJSON(t, srv, "/api/v1/instance-settings", map[string]string{
		"recording_enabled":     "true",
		"recording_sample_rate": "0.5",
	}, nil)
	if wp.Code != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d (body: %s)", wp.Code, wp.Body.String())
	}
	var resp struct {
		Settings map[string]string `json:"settings"`
	}
	_ = json.Unmarshal(wp.Body.Bytes(), &resp)
	if resp.Settings["recording_enabled"] != "true" {
		t.Errorf("recording_enabled: want true, got %q", resp.Settings["recording_enabled"])
	}

	// GET returns persisted values.
	wg := getJSON(t, srv, "/api/v1/instance-settings", nil)
	if wg.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d", wg.Code)
	}
}

func TestHandlePutInstanceSettings_BadJSON(t *testing.T) {
	srv, _ := fullServer(t, func(cfg *ServerConfig) { cfg.InstanceSettings = storeInstanceSettings(cfg) })
	w := putRaw(t, srv, "/api/v1/instance-settings", "{not json}")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad json, got %d (body: %s)", w.Code, w.Body.String())
	}
}
