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
		ProfileURL            string `json:"profile_url,omitempty"`
		ServerURL             string `json:"server_url,omitempty"`
		ClientID              string `json:"client_id,omitempty"`
		WidgetURL             string `json:"widget_url,omitempty"`
		PostLogoutRedirectURI string `json:"post_logout_redirect_uri,omitempty"`
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

func TestClientConfig_ExposesWidgetFields(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.iambarnProvider = iambarn.New("https://iam.test.wiebe.xyz/", "ibc_test123", "https://funnelbarn.test/cb")
	srv.postLogoutRedirect = "https://funnelbarn.test/api/v1/auth/oidc/logged-out"

	got := getClientConfig(t, srv)
	if got.IAMBarn.ServerURL != "https://iam.test.wiebe.xyz" {
		t.Errorf("server_url: got %q", got.IAMBarn.ServerURL)
	}
	if got.IAMBarn.ClientID != "ibc_test123" {
		t.Errorf("client_id: got %q", got.IAMBarn.ClientID)
	}
	if got.IAMBarn.WidgetURL != "https://iam.test.wiebe.xyz/widget/iambarn-widget.iife.js" {
		t.Errorf("widget_url: got %q", got.IAMBarn.WidgetURL)
	}
	if got.IAMBarn.PostLogoutRedirectURI != "https://funnelbarn.test/api/v1/auth/oidc/logged-out" {
		t.Errorf("post_logout_redirect_uri: got %q", got.IAMBarn.PostLogoutRedirectURI)
	}
}

func TestClientConfig_NoWidgetFieldsWhenIAMBarnUnconfigured(t *testing.T) {
	srv, _ := newTestServer(t)
	got := getClientConfig(t, srv)
	if got.IAMBarn.ServerURL != "" || got.IAMBarn.ClientID != "" || got.IAMBarn.WidgetURL != "" {
		t.Errorf("expected no widget fields, got %+v", got.IAMBarn)
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
