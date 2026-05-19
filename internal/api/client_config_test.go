package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/iambarn"
)

type clientConfigResp struct {
	IAMBarnEnabled bool `json:"iambarn_enabled"`
	IAMBarn        struct {
		ProfileURL string `json:"profile_url,omitempty"`
	} `json:"iambarn,omitempty"`
	OIDC struct {
		Enabled  bool   `json:"enabled"`
		LoginURL string `json:"loginURL,omitempty"`
	} `json:"oidc"`
}

func getClientConfig(t *testing.T, srv *Server) clientConfigResp {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/client-config", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out clientConfigResp
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	return out
}

func TestClientConfig_NoIAMBarnConfigured(t *testing.T) {
	srv, _ := newTestServer(t)
	got := getClientConfig(t, srv)
	if got.IAMBarn.ProfileURL != "" {
		t.Errorf("expected empty profile_url, got %q", got.IAMBarn.ProfileURL)
	}
}

func TestClientConfig_LegacyIAMBarnProviderSetsProfileURL(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.iambarnProvider = iambarn.New("https://iam.test.wiebe.xyz/", "test-client", "https://funnelbarn.test/cb")
	got := getClientConfig(t, srv)
	want := "https://iam.test.wiebe.xyz/admin#profile"
	if got.IAMBarn.ProfileURL != want {
		t.Errorf("profile_url: got %q, want %q", got.IAMBarn.ProfileURL, want)
	}
}

func TestClientConfig_ConfidentialOIDCSetsProfileURL(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.oidc = auth.NewOIDCClient(auth.OIDCConfig{
		Issuer:       "https://iam.staging.wiebe.xyz",
		ClientID:     "fb",
		ClientSecret: "secret",
		RedirectURL:  "https://funnelbarn.staging/cb",
	})
	got := getClientConfig(t, srv)
	want := "https://iam.staging.wiebe.xyz/admin#profile"
	if got.IAMBarn.ProfileURL != want {
		t.Errorf("profile_url: got %q, want %q", got.IAMBarn.ProfileURL, want)
	}
	if !got.OIDC.Enabled {
		t.Errorf("expected oidc.enabled = true")
	}
}
