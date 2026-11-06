package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// refreshIssuer answers the refresh grant like IAMBarn: a rotated pair and no
// id_token. accessToken builds the access token from the issuer URL.
func refreshIssuer(t *testing.T, accessToken func(it *testIssuer) string) *testIssuer {
	t.Helper()
	var it *testIssuer
	it = newTestIssuer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  accessToken(it),
			"refresh_token": "new-refresh",
			"token_type":    "Bearer",
			"expires_in":    900,
		})
	}, nil)
	return it
}

func TestOIDCClientRefreshReadsClaimsFromTheAccessToken(t *testing.T) {
	it := refreshIssuer(t, func(it *testIssuer) string {
		return it.sign(t, map[string]any{
			"iss": it.srv.URL, "aud": []string{"funnelbarn-web"}, "sub": "u1",
			"exp": time.Now().Add(15 * time.Minute).Unix(), "iat": time.Now().Unix(),
			"organization_id": "org-home", "roles": []string{"member"}, "token_use": "access_token",
		})
	})
	result, err := it.client().Refresh(context.Background(), "old-refresh")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if result.IDToken != "" {
		t.Errorf("IDToken = %q, want empty (no id_token in the response)", result.IDToken)
	}
	if result.ClaimsSource != "access_token" {
		t.Errorf("ClaimsSource = %q, want access_token", result.ClaimsSource)
	}
	if result.Claims == nil {
		t.Fatal("expected claims from the access token")
	}
	if result.Claims.Subject != "u1" || result.Claims.OrganizationID != "org-home" ||
		len(result.Claims.Roles) != 1 || result.Claims.Roles[0] != "member" {
		t.Errorf("claims = %+v", result.Claims)
	}
}

func TestOIDCClientRefreshKeepsNoClaimsForAnUnverifiableAccessToken(t *testing.T) {
	cases := map[string]func(it *testIssuer) string{
		"opaque": func(*testIssuer) string { return "opaque-access" },
		"minted for another audience": func(it *testIssuer) string {
			return it.sign(t, map[string]any{
				"iss": it.srv.URL, "aud": []string{"https://other.example.com"}, "sub": "u1",
				"exp": time.Now().Add(15 * time.Minute).Unix(), "iat": time.Now().Unix(),
				"organization_id": "org-home", "roles": []string{"owner"},
			})
		},
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			result, err := refreshIssuer(t, tok).client().Refresh(context.Background(), "old-refresh")
			if err != nil {
				t.Fatalf("Refresh: %v", err)
			}
			if result.Claims != nil || result.ClaimsSource != "none" {
				t.Errorf("Claims = %+v (source %q), want nil and none so the caller keeps its snapshot",
					result.Claims, result.ClaimsSource)
			}
			if result.RefreshToken != "new-refresh" {
				t.Errorf("RefreshToken = %q, want the rotated one", result.RefreshToken)
			}
		})
	}
}

// An access token without a roles claim takes its roles from userinfo, as
// VerifyAccessToken does for the MCP. A userinfo failure gives no claims, so
// an outage keeps the earlier snapshot and logs nobody out.
func TestOIDCClientRefreshTakesRolesFromUserinfoWhenTheTokenHasNone(t *testing.T) {
	noRoles := func(it *testIssuer) string {
		return it.sign(t, map[string]any{
			"iss": it.srv.URL, "aud": []string{"funnelbarn-web"}, "sub": "u1",
			"exp": time.Now().Add(15 * time.Minute).Unix(), "iat": time.Now().Unix(),
			"organization_id": "org-home", "token_use": "access_token",
		})
	}
	cases := []struct {
		name      string
		userinfo  http.HandlerFunc
		wantRoles []string // nil: no claims at all
	}{
		{"userinfo has the role", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"sub": "u1", "roles": []string{"organization_admin"}})
		}, []string{"organization_admin"}},
		{"userinfo lacks the role", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"sub": "u1", "roles": []string{"member"}})
		}, []string{"member"}},
		{"userinfo fails", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "down", http.StatusBadGateway)
		}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			it := refreshIssuer(t, noRoles)
			var calls int
			it.mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
				calls++
				tc.userinfo(w, r)
			})
			result, err := it.client().Refresh(context.Background(), "old-refresh")
			if err != nil {
				t.Fatalf("Refresh: %v", err)
			}
			if calls != 1 {
				t.Errorf("userinfo calls = %d, want 1", calls)
			}
			if tc.wantRoles == nil {
				if result.Claims != nil {
					t.Errorf("Claims = %+v, want nil so the caller keeps its snapshot", result.Claims)
				}
				return
			}
			if result.Claims == nil || result.Claims.OrganizationID != "org-home" ||
				strings.Join(result.Claims.Roles, ",") != strings.Join(tc.wantRoles, ",") {
				t.Errorf("claims = %+v, want roles %v", result.Claims, tc.wantRoles)
			}
		})
	}
}
