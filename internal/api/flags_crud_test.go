package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// sessionAndCSRF mints a session (cookie + server-side row) + matching CSRF
// for a server with auth enabled. Both must come from the same token (server
// derives CSRF from cookie.Value with the session secret).
func sessionAndCSRF(t *testing.T, srv *Server, username string) (*http.Cookie, string) {
	t.Helper()
	cookie := sessionCookieFor(t, srv, username)
	return cookie, srv.sessionManager.CSRFToken(cookie.Value)
}

func TestHandleListFlags_Empty(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "Empty", "empty")
	cookie, _ := sessionAndCSRF(t, srv, "u")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/flags", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("list flags: want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	flags, ok := resp["flags"].([]any)
	if !ok {
		t.Fatalf("response missing flags array: %v", resp)
	}
	if len(flags) != 0 {
		t.Errorf("expected empty flags list, got %d", len(flags))
	}
}

func TestHandleListFlags_ReturnsCreated(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "WithFlags", "withflags")
	_, _ = store.CreateFlag(ctx, repository.FeatureFlag{
		ProjectID:      p.ID,
		FlagKey:        "feature_x",
		Name:           "Feature X",
		FlagType:       "release",
		Variants:       `{"on":true,"off":false}`,
		DefaultVariant: "off",
		Split:          `{"on":50,"off":50}`,
		Status:         "active",
	})
	cookie, _ := sessionAndCSRF(t, srv, "u")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/flags", cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("list flags: want 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	flags := resp["flags"].([]any)
	if len(flags) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(flags))
	}
}

func TestHandleCreateFlag(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "CreateFlag", "createflag")
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	// The flag handler takes variants/split as already-JSON-encoded strings.
	w := postJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags", map[string]any{
		"flag_key":        "new_flag",
		"name":            "New Flag",
		"flag_type":       "release",
		"variants":        `{"on":true,"off":false}`,
		"default_variant": "off",
		"split":           `{"on":0,"off":100}`,
	}, cookie, csrf)

	if w.Code != http.StatusCreated {
		t.Fatalf("create flag: want 201, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["flag_key"] != "new_flag" {
		t.Errorf("flag_key: got %v", resp["flag_key"])
	}
}

func TestHandleCreateFlag_MissingKey_Returns422(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "BadFlag", "badflag")
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	w := postJSONWithCSRF(t, srv, "/api/v1/projects/"+p.ID+"/flags", map[string]any{
		"name": "no key",
	}, cookie, csrf)

	if w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusBadRequest {
		t.Errorf("expected 4xx for missing flag_key, got %d (body: %s)", w.Code, w.Body.String())
	}
}

func TestHandleGetFlag(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "GetFlag", "getflag")
	flag, _ := store.CreateFlag(ctx, repository.FeatureFlag{
		ProjectID:      p.ID,
		FlagKey:        "get_me",
		Name:           "Get Me",
		FlagType:       "release",
		Variants:       `{"on":true}`,
		DefaultVariant: "on",
		Split:          `{"on":100}`,
		Status:         "active",
	})
	cookie, _ := sessionAndCSRF(t, srv, "u")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/flags/"+flag.ID, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("get flag: want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["id"] != flag.ID {
		t.Errorf("id: want %s, got %v", flag.ID, resp["id"])
	}
}

func TestHandleGetFlag_NotFound(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "MissingFlag", "missingflag")
	cookie, _ := sessionAndCSRF(t, srv, "u")

	w := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/flags/does-not-exist", cookie)
	if w.Code != http.StatusNotFound {
		t.Errorf("get missing flag: want 404, got %d", w.Code)
	}
}

func TestHandleDeleteFlag(t *testing.T) {
	srv, store := newAuthedServer(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, "DeleteFlag", "deleteflag")
	flag, _ := store.CreateFlag(ctx, repository.FeatureFlag{
		ProjectID:      p.ID,
		FlagKey:        "kill_me",
		Name:           "Kill Me",
		FlagType:       "release",
		Variants:       `{"on":true}`,
		DefaultVariant: "on",
		Split:          `{"on":100}`,
		Status:         "active",
	})
	cookie, csrf := sessionAndCSRF(t, srv, "u")

	// DELETE with CSRF header set on the request manually (deleteReq doesn't take one).
	req, _ := http.NewRequest(http.MethodDelete, "/api/v1/projects/"+p.ID+"/flags/"+flag.ID, nil)
	req.AddCookie(cookie)
	req.Header.Set("X-FunnelBarn-CSRF", csrf)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent && w.Code != http.StatusOK {
		t.Errorf("delete flag: want 2xx, got %d (body: %s)", w.Code, w.Body.String())
	}

	// Confirm it's gone.
	w2 := getJSON(t, srv, "/api/v1/projects/"+p.ID+"/flags/"+flag.ID, cookie)
	if w2.Code != http.StatusNotFound {
		t.Errorf("after delete: want 404, got %d", w2.Code)
	}
}
