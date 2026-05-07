package api

import (
	"net/http"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
)

func TestCSRF_MutatingRequestWithoutToken_Blocked(t *testing.T) {
	srv, _ := newAuthedServer(t)
	cookie := sessionCookieFor(t, srv, "admin")

	w := postJSON(t, srv, "/api/v1/projects", map[string]string{
		"name": "CSRF Test",
		"slug": "csrf-test",
	}, cookie)
	if w.Code != http.StatusForbidden {
		t.Errorf("POST without CSRF token: want 403, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestCSRF_MutatingRequestWithValidToken_Allowed(t *testing.T) {
	srv, _ := newAuthedServer(t)
	token, expires, err := srv.sessionManager.Create("admin")
	if err != nil {
		t.Fatal(err)
	}
	cookie := auth.SessionCookie(token, expires, false)
	csrfToken := auth.CSRFToken(token)

	w := postJSONWithCSRF(t, srv, "/api/v1/projects", map[string]string{
		"name": "CSRF OK",
		"slug": "csrf-ok",
	}, cookie, csrfToken)
	if w.Code != http.StatusCreated {
		t.Errorf("POST with valid CSRF: want 201, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestCSRF_MutatingRequestWithWrongToken_Blocked(t *testing.T) {
	srv, _ := newAuthedServer(t)
	cookie := sessionCookieFor(t, srv, "admin")

	w := postJSONWithCSRF(t, srv, "/api/v1/projects", map[string]string{
		"name": "CSRF Bad",
		"slug": "csrf-bad",
	}, cookie, "wrong-token")
	if w.Code != http.StatusForbidden {
		t.Errorf("POST with wrong CSRF: want 403, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestCSRF_GETRequestWithoutToken_Allowed(t *testing.T) {
	srv, _ := newAuthedServer(t)
	cookie := sessionCookieFor(t, srv, "admin")

	w := getJSON(t, srv, "/api/v1/projects", cookie)
	if w.Code != http.StatusOK {
		t.Errorf("GET without CSRF: want 200, got %d", w.Code)
	}
}

func TestCSRF_NoAuthConfigured_Skipped(t *testing.T) {
	srv, _ := newTestServer(t)

	w := postJSON(t, srv, "/api/v1/projects", map[string]string{
		"name": "No Auth",
		"slug": "no-auth",
	}, nil)
	if w.Code != http.StatusCreated {
		t.Errorf("POST without auth: want 201, got %d (body: %s)", w.Code, w.Body.String())
	}
}
