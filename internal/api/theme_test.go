package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestThemeManifest(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/iambarn-theme.json", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d (body=%q)", w.Code, http.StatusOK, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type: got %q want it to contain application/json", ct)
	}

	type manifest struct {
		Name            string `json:"name"`
		LogoURL         string `json:"logo_url"`
		PrimaryColor    string `json:"primary_color"`
		BackgroundColor string `json:"background_color"`
		CardColor       string `json:"card_color"`
		BodyTextColor   string `json:"body_text_color"`
		SupportURL      string `json:"support_url"`
		Locale          string `json:"locale"`
	}
	var m manifest
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if m.Name == "" {
		t.Errorf("name should be populated, got empty")
	}
}
