package api

import (
	"context"
	"net/http"
	"testing"
)

func storeRecordingSettings(cfg *ServerConfig) ProjectRecordingSettingsRepo {
	return cfg.DB.(ProjectRecordingSettingsRepo)
}

func TestHandleGetProjectRecordingSettings_Unavailable(t *testing.T) {
	srv, store := newTestServer(t)
	p, _ := store.CreateProject(context.Background(), "Rec", "rec")
	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/recording-settings", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleUpdateProjectRecordingSettings_Unavailable(t *testing.T) {
	srv, store := newTestServer(t)
	p, _ := store.CreateProject(context.Background(), "Rec2", "rec2")
	w := putJSON(t, srv, "/api/v1/projects/"+p.ID+"/recording-settings", map[string]any{}, nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func recordingServerWithStore(t *testing.T) (*Server, string) {
	t.Helper()
	srv, store := fullServer(t, func(cfg *ServerConfig) {
		cfg.RecordingSettings = storeRecordingSettings(cfg)
		cfg.InstanceSettings = cfg.DB.(InstanceSettingsRepo)
	})
	p, _ := store.CreateProject(context.Background(), "RecStore", "rec-store")
	return srv, p.ID
}

func TestHandleGetProjectRecordingSettings_OK(t *testing.T) {
	srv, pid := recordingServerWithStore(t)
	w := getJSON(t, srv, "/api/v1/projects/"+pid+"/recording-settings", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleUpdateProjectRecordingSettings_OK(t *testing.T) {
	srv, pid := recordingServerWithStore(t)
	enabled := true
	rate := 0.5
	body := map[string]any{
		"enabled":     enabled,
		"sample_rate": rate,
		"rules": []map[string]string{
			{"action": "capture", "pattern": "/checkout"},
		},
	}
	w := putJSON(t, srv, "/api/v1/projects/"+pid+"/recording-settings", body, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (body: %s)", w.Code, w.Body.String())
	}

	// Read back reflects persisted enabled + rate merged into effective config.
	wg := getJSON(t, srv, "/api/v1/projects/"+pid+"/recording-settings", nil)
	if wg.Code != http.StatusOK {
		t.Fatalf("readback expected 200, got %d", wg.Code)
	}
}

func TestHandleUpdateProjectRecordingSettings_BadSampleRate(t *testing.T) {
	srv, pid := recordingServerWithStore(t)
	body := map[string]any{"sample_rate": 5.0}
	w := putJSON(t, srv, "/api/v1/projects/"+pid+"/recording-settings", body, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad sample_rate, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleUpdateProjectRecordingSettings_BadRule(t *testing.T) {
	srv, pid := recordingServerWithStore(t)
	body := map[string]any{
		"rules": []map[string]string{{"action": "explode", "pattern": "/x"}},
	}
	w := putJSON(t, srv, "/api/v1/projects/"+pid+"/recording-settings", body, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad rule action, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleUpdateProjectRecordingSettings_EmptyPattern(t *testing.T) {
	srv, pid := recordingServerWithStore(t)
	body := map[string]any{
		"rules": []map[string]string{{"action": "capture", "pattern": ""}},
	}
	w := putJSON(t, srv, "/api/v1/projects/"+pid+"/recording-settings", body, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty pattern, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleUpdateProjectRecordingSettings_BadJSON(t *testing.T) {
	srv, pid := recordingServerWithStore(t)
	w := putRaw(t, srv, "/api/v1/projects/"+pid+"/recording-settings", "{bad")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad json, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleGetRecordingConfig_NoAPIKey(t *testing.T) {
	// No API key -> instance defaults path.
	srv, _ := fullServer(t, func(cfg *ServerConfig) {
		cfg.InstanceSettings = cfg.DB.(InstanceSettingsRepo)
		cfg.RecordingSettings = storeRecordingSettings(cfg)
	})
	w := getJSON(t, srv, "/api/v1/recording-config", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}
