package api

import (
	"context"
	"net/http"
	"testing"
)

// store2Geo returns the config's DB (a memory repository.Store) as a
// GeoAnonymizer — the store implements the interface directly.
func store2Geo(cfg *ServerConfig) GeoAnonymizer { return cfg.DB.(GeoAnonymizer) }

func TestHandleAnonymizeGeo_Unavailable(t *testing.T) {
	// newTestServer wires no GeoAnonymizer -> 503.
	srv, _ := newTestServer(t)
	w := postJSON(t, srv, "/api/v1/admin/anonymize-geo", map[string]string{"ip": "1.2.3.4"}, nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleAnonymizeGeo_MissingFields(t *testing.T) {
	srv, _ := fullServer(t, func(cfg *ServerConfig) { cfg.GeoAnonymizer = store2Geo(cfg) })
	w := postJSON(t, srv, "/api/v1/admin/anonymize-geo", map[string]string{}, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleAnonymizeGeo_InvalidIP(t *testing.T) {
	srv, _ := fullServer(t, func(cfg *ServerConfig) { cfg.GeoAnonymizer = store2Geo(cfg) })
	w := postJSON(t, srv, "/api/v1/admin/anonymize-geo", map[string]string{"ip": "not-an-ip"}, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for invalid ip, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleAnonymizeGeo_HappyPath(t *testing.T) {
	srv, store := fullServer(t, func(cfg *ServerConfig) { cfg.GeoAnonymizer = store2Geo(cfg) })
	_, _ = store.CreateProject(context.Background(), "Geo", "geo")

	// Both session_id and a valid ip -> two anonymize calls, 200.
	w := postJSON(t, srv, "/api/v1/admin/anonymize-geo", map[string]string{
		"session_id": "sess-1",
		"ip":         "203.0.113.9",
	}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
}
