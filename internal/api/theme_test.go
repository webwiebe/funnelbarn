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

		DefaultLocale    string   `json:"default_locale"`
		SupportedLocales []string `json:"supported_locales"`
		FromAddress      string   `json:"from_address"`
		FromName         string   `json:"from_name"`
		Dark             *struct {
			PrimaryColor    string `json:"primary_color"`
			BackgroundColor string `json:"background_color"`
			CardColor       string `json:"card_color"`
			BodyTextColor   string `json:"body_text_color"`
		} `json:"dark"`
	}
	var m manifest
	if err := json.NewDecoder(w.Body).Decode(&m); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if m.Name == "" {
		t.Errorf("name should be populated, got empty")
	}
	if m.FromAddress == "" || m.FromName == "" {
		t.Errorf("from_address/from_name must be set so auth mail is not sent from a derived sender, got %q/%q", m.FromAddress, m.FromName)
	}
	if m.DefaultLocale == "" || len(m.SupportedLocales) == 0 {
		t.Errorf("default_locale/supported_locales must be set, got %q/%v", m.DefaultLocale, m.SupportedLocales)
	}
	if m.Dark == nil || m.Dark.BackgroundColor == "" {
		t.Errorf("dark palette must be set, got %+v", m.Dark)
	}
}
