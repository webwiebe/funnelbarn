package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
)

type clientConfigResp struct {
	IAMBarn struct {
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

func testOIDCClientConfig(issuer string) *auth.OIDCClient {
	return auth.NewOIDCClient(auth.OIDCConfig{
		Issuer:       issuer,
		ClientID:     "fb_conf_client",
		ClientSecret: "secret",
		RedirectURL:  "https://funnelbarn.test/api/v1/oidc/callback",
	})
}

func TestClientConfig_NoOIDCConfigured(t *testing.T) {
	srv, _ := newTestServer(t)
	got := getClientConfig(t, srv)
	if got.IAMBarn.ProfileURL != "" || got.IAMBarn.ServerURL != "" || got.IAMBarn.ClientID != "" || got.IAMBarn.WidgetURL != "" {
		t.Errorf("expected no iambarn fields without OIDC, got %+v", got.IAMBarn)
	}
	if got.OIDC.Enabled {
		t.Error("expected oidc.enabled = false")
	}
}

// TestClientConfig_ExposesWidgetFields verifies the hosted IAMBarn widget
// still gets issuer + client_id + post-logout URI from client-config now that
// they derive from the confidential OIDC client (bugbarn feeds a confidential
// client id to the same widget).
func TestClientConfig_ExposesWidgetFields(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.oidc = testOIDCClientConfig("https://iam.test.wiebe.xyz/")
	srv.postLogoutRedirect = "https://funnelbarn.test/api/v1/auth/oidc/logged-out"

	got := getClientConfig(t, srv)
	if got.IAMBarn.ServerURL != "https://iam.test.wiebe.xyz" {
		t.Errorf("server_url: got %q", got.IAMBarn.ServerURL)
	}
	if got.IAMBarn.ClientID != "fb_conf_client" {
		t.Errorf("client_id: got %q", got.IAMBarn.ClientID)
	}
	if got.IAMBarn.WidgetURL != "https://iam.test.wiebe.xyz/widget/iambarn-widget.iife.js" {
		t.Errorf("widget_url: got %q", got.IAMBarn.WidgetURL)
	}
	if got.IAMBarn.PostLogoutRedirectURI != "https://funnelbarn.test/api/v1/auth/oidc/logged-out" {
		t.Errorf("post_logout_redirect_uri: got %q", got.IAMBarn.PostLogoutRedirectURI)
	}
}

func TestClientConfig_OIDCSetsProfileURLAndLoginURL(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.oidc = testOIDCClientConfig("https://iam.staging.wiebe.xyz")
	got := getClientConfig(t, srv)
	if want := "https://iam.staging.wiebe.xyz/admin#profile"; got.IAMBarn.ProfileURL != want {
		t.Errorf("profile_url: got %q, want %q", got.IAMBarn.ProfileURL, want)
	}
	if !got.OIDC.Enabled {
		t.Error("expected oidc.enabled = true")
	}
	if got.OIDC.LoginURL != "/api/v1/oidc/login" {
		t.Errorf("loginURL: got %q", got.OIDC.LoginURL)
	}
}
