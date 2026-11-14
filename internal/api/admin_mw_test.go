package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// adminFixture is an OIDC-capable server with one project and one stored key.
type adminFixture struct {
	srv   *Server
	store *repository.Store
	f     *fakeIdP
	proj  domain.Project
	key   domain.APIKey
}

func newAdminFixture(t *testing.T) adminFixture {
	t.Helper()
	f := newFakeIdP(t)
	srv, store := oidcFlowServer(t, f, nil)
	ctx := context.Background()
	p, err := store.CreateProject(ctx, "Keys", "keys")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	k, err := store.CreateAPIKey(ctx, "existing", p.ID, strings.Repeat("a", 64), domain.APIKeyScopeIngest)
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	return adminFixture{srv: srv, store: store, f: f, proj: p, key: k}
}

// oidcSession seeds a live OIDC session whose stored claims are claimsJSON and
// returns its cookie and CSRF token.
func (fx adminFixture) oidcSession(t *testing.T, claimsJSON string) (*http.Cookie, string) {
	t.Helper()
	cookie, _ := insertOIDCSession(t, fx.srv, domain.WebSession{
		Username:        "alice",
		IdpSub:          fakeIdPSub,
		IdpSid:          fakeIdPSid,
		AccessToken:     "at",
		RefreshToken:    "rt",
		AccessExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
		ClaimsJSON:      claimsJSON,
	})
	return cookie, fx.srv.sessionManager.CSRFToken(cookie.Value)
}

func (fx adminFixture) keyCount(t *testing.T) int {
	t.Helper()
	keys, err := fx.store.ListAllAPIKeys(context.Background())
	if err != nil {
		t.Fatalf("ListAllAPIKeys: %v", err)
	}
	return len(keys)
}

// call runs one of the three key routes with the given session.
func (fx adminFixture) call(t *testing.T, route string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	switch route {
	case "list":
		return getJSON(t, fx.srv, "/api/v1/apikeys", cookie)
	case "create":
		return postJSONWithCSRF(t, fx.srv, "/api/v1/apikeys",
			map[string]string{"name": "new", "project_id": fx.proj.ID, "scope": "full"}, cookie, csrf)
	case "delete":
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/apikeys/"+fx.key.ID, nil)
		req.AddCookie(cookie)
		req.Header.Set("X-FunnelBarn-CSRF", csrf)
		w := httptest.NewRecorder()
		fx.srv.ServeHTTP(w, req)
		return w
	}
	t.Fatalf("unknown route %q", route)
	return nil
}

var (
	ownerClaims  = `{"sub":"user-sub-1","organization_id":"` + fakeIdPOrgID + `","roles":["owner"]}`
	memberClaims = `{"sub":"user-sub-1","organization_id":"` + fakeIdPOrgID + `","roles":["member"],"groups":["funnelbarn-users"]}`
)

// A session whose stored claims carry no admin role gets 403 on every key
// route, and nothing changes.
func TestAPIKeyRoutes_RefuseASessionWithoutAnAdminRole(t *testing.T) {
	claims := map[string]string{
		"member of this organization":   memberClaims,
		"owner of another organization": `{"sub":"user-sub-1","organization_id":"org-other","roles":["owner"]}`,
		"group only":                    `{"sub":"user-sub-1","organization_id":"` + fakeIdPOrgID + `","groups":["funnelbarn-admins"]}`,
		"no stored claims":              ``,
		"unreadable claims":             `{not json`,
	}
	for _, route := range []string{"list", "create", "delete"} {
		for name, c := range claims {
			t.Run(route+"/"+name, func(t *testing.T) {
				fx := newAdminFixture(t)
				cookie, csrf := fx.oidcSession(t, c)
				w := fx.call(t, route, cookie, csrf)
				if w.Code != http.StatusForbidden {
					t.Fatalf("%s = %d, want 403 (body: %s)", route, w.Code, w.Body.String())
				}
				if strings.Contains(w.Body.String(), fx.key.ID) {
					t.Errorf("403 body leaks key data: %s", w.Body.String())
				}
				if got := fx.keyCount(t); got != 1 {
					t.Errorf("key count = %d after a refused %s, want 1", got, route)
				}
			})
		}
	}
}

// The roles that grant sign-in also grant the key routes.
func TestAPIKeyRoutes_AllowAnAdminRole(t *testing.T) {
	want := map[string]int{"list": http.StatusOK, "create": http.StatusCreated, "delete": http.StatusNoContent}
	for _, role := range []string{"owner", "organization_admin", "operator"} {
		for route, code := range want {
			t.Run(route+"/"+role, func(t *testing.T) {
				fx := newAdminFixture(t)
				cookie, csrf := fx.oidcSession(t, `{"sub":"user-sub-1","organization_id":"`+fakeIdPOrgID+`","roles":["`+role+`"]}`)
				if w := fx.call(t, route, cookie, csrf); w.Code != code {
					t.Fatalf("%s = %d, want %d (body: %s)", route, w.Code, code, w.Body.String())
				}
			})
		}
	}
}

// Local sessions (env-var admin and DB users) keep the key routes.
func TestAPIKeyRoutes_AllowALocalSession(t *testing.T) {
	want := map[string]int{"list": http.StatusOK, "create": http.StatusCreated, "delete": http.StatusNoContent}
	for route, code := range want {
		t.Run(route, func(t *testing.T) {
			fx := newAdminFixture(t)
			cookie, csrf := sessionAndCSRF(t, fx.srv, "admin")
			if w := fx.call(t, route, cookie, csrf); w.Code != code {
				t.Fatalf("%s = %d, want %d (body: %s)", route, w.Code, code, w.Body.String())
			}
		})
	}
}

// When a refresh returns claims without the role, key access ends at that
// refresh: the refresh re-checks the sign-in role and ends the session, so
// the key route answers 401 and the session stays gone.
func TestAPIKeyRoutes_RefreshedClaimsWithoutTheRoleEndAccess(t *testing.T) {
	fx := newAdminFixture(t)
	cookie, _ := insertOIDCSession(t, fx.srv, domain.WebSession{
		Username:        "alice",
		IdpSub:          fakeIdPSub,
		IdpSid:          fakeIdPSid,
		AccessToken:     "at-old",
		RefreshToken:    "rt-old",
		AccessExpiresAt: time.Now().Add(-time.Minute).Unix(),
		ClaimsJSON:      ownerClaims,
	})
	fx.f.setTokenHandler(func(tw http.ResponseWriter, _ *http.Request) {
		tokenResponse(tw, "at-new", "rt-new", fx.f.idToken(t, "", map[string]any{"roles": []string{"member"}}), 900)
	})
	for i := 0; i < 2; i++ {
		if w := fx.call(t, "list", cookie, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("list %d after demotion = %d, want 401 (body: %s)", i+1, w.Code, w.Body.String())
		}
	}
}
