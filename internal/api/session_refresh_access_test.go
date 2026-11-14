package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// accessToken builds an IAMBarn-style access token for the dashboard client.
func (f *fakeIdP) accessToken(t *testing.T, extra map[string]any) string {
	t.Helper()
	claims := map[string]any{
		"iss":             f.srv.URL,
		"sub":             fakeIdPSub,
		"aud":             []string{fakeIdPClientID},
		"exp":             time.Now().Add(15 * time.Minute).Unix(),
		"iat":             time.Now().Unix(),
		"organization_id": fakeIdPOrgID,
		"roles":           []string{"owner"},
		"token_use":       "access_token",
	}
	for k, v := range extra {
		claims[k] = v
	}
	return f.sign(t, claims)
}

// expiredOwnerSession seeds an OIDC session whose access token has expired, so
// the next request runs the refresh grant.
func expiredOwnerSession(t *testing.T, srv *Server) (*http.Cookie, string) {
	t.Helper()
	return insertOIDCSession(t, srv, domain.WebSession{
		Username:        "alice",
		IdpSub:          fakeIdPSub,
		IdpSid:          fakeIdPSid,
		AccessToken:     "at-old",
		RefreshToken:    "rt-old",
		AccessExpiresAt: time.Now().Add(-time.Minute).Unix(),
		ClaimsJSON:      `{"sub":"user-sub-1","organization_id":"` + fakeIdPOrgID + `","roles":["owner"]}`,
	})
}

// A refresh whose claims no longer pass the sign-in check ends the session on
// every route and revokes the refresh token the IdP just issued (#481).
func TestSessionRefresh_RoleGoneEndsTheSession(t *testing.T) {
	cases := map[string]func(f *fakeIdP, w http.ResponseWriter){
		"access token without the role, no id_token": func(f *fakeIdP, w http.ResponseWriter) {
			tokenResponse(w, f.accessToken(t, map[string]any{"roles": []string{"member"}}), "rt-new", "", 900)
		},
		"access token from another organization": func(f *fakeIdP, w http.ResponseWriter) {
			tokenResponse(w, f.accessToken(t, map[string]any{"organization_id": "org-other"}), "rt-new", "", 900)
		},
		"id_token without the role": func(f *fakeIdP, w http.ResponseWriter) {
			tokenResponse(w, "at-new", "rt-new", f.idToken(t, "", map[string]any{"roles": []string{"member"}}), 900)
		},
	}
	for name, respond := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFakeIdP(t)
			srv, store := oidcFlowServer(t, f, nil)
			cookie, idHash := expiredOwnerSession(t, srv)
			f.setTokenHandler(func(w http.ResponseWriter, _ *http.Request) { respond(f, w) })

			if w := getJSON(t, srv, "/api/v1/me", cookie); w.Code != http.StatusUnauthorized {
				t.Fatalf("/me after the role went = %d, want 401 (body: %s)", w.Code, w.Body.String())
			}
			if _, err := store.GetWebSession(context.Background(), idHash); err == nil {
				t.Error("session row still present after the role went")
			}
			revs := f.revocations()
			if len(revs) != 1 || revs[0].Get("token") != "rt-new" {
				t.Errorf("revocations = %v, want one for rt-new", revs)
			}
			if w := getJSON(t, srv, "/api/v1/me", cookie); w.Code != http.StatusUnauthorized {
				t.Errorf("second request = %d, want 401", w.Code)
			}
		})
	}
}

// IAMBarn's refresh carries no id_token; the access token's claims replace the
// stored snapshot when the role is still held.
func TestSessionRefresh_AccessTokenClaimsReplaceTheSnapshot(t *testing.T) {
	logs := captureSlog(t)
	f := newFakeIdP(t)
	srv, store := oidcFlowServer(t, f, nil)
	cookie, idHash := expiredOwnerSession(t, srv)
	f.setTokenHandler(func(w http.ResponseWriter, _ *http.Request) {
		tokenResponse(w, f.accessToken(t, map[string]any{"roles": []string{"organization_admin"}}), "rt-new", "", 900)
	})

	if w := getJSON(t, srv, "/api/v1/me", cookie); w.Code != http.StatusOK {
		t.Fatalf("/me = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	ws, err := store.GetWebSession(context.Background(), idHash)
	if err != nil {
		t.Fatalf("row gone: %v", err)
	}
	if !strings.Contains(ws.ClaimsJSON, "organization_admin") {
		t.Errorf("claims not replaced from the access token: %q", ws.ClaimsJSON)
	}
	if ws.RefreshToken != "rt-new" || len(f.revocations()) != 0 {
		t.Errorf("refresh token = %q, revocations = %v", ws.RefreshToken, f.revocations())
	}
	// The post-deploy check reads this record from SpanBarn logs.
	if got := rotatedLogSources(logs); len(got) != 1 || got[0] != "access_token" {
		t.Errorf("rotated log claims_source = %v, want [access_token]", got)
	}
}

// rotatedLogSources returns the claims_source of each Info rotation record.
func rotatedLogSources(h *recordingHandler) []any {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []any
	for _, r := range h.recs {
		if r.Level == slog.LevelInfo && r.Message == "session refresh: rotated" {
			out = append(out, recordAttrs(r)["claims_source"])
		}
	}
	return out
}

// A refresh with no verifiable claims keeps the session and its snapshot.
func TestSessionRefresh_NoClaimsKeepsTheSnapshot(t *testing.T) {
	f := newFakeIdP(t)
	srv, store := oidcFlowServer(t, f, nil)
	cookie, idHash := expiredOwnerSession(t, srv)
	f.setTokenHandler(func(w http.ResponseWriter, _ *http.Request) {
		tokenResponse(w, "opaque-access", "rt-new", "", 900)
	})

	if w := getJSON(t, srv, "/api/v1/me", cookie); w.Code != http.StatusOK {
		t.Fatalf("/me = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	ws, err := store.GetWebSession(context.Background(), idHash)
	if err != nil {
		t.Fatalf("row gone: %v", err)
	}
	if !strings.Contains(ws.ClaimsJSON, `"owner"`) || ws.RefreshToken != "rt-new" {
		t.Errorf("snapshot or tokens wrong: claims=%q refresh=%q", ws.ClaimsJSON, ws.RefreshToken)
	}
}
