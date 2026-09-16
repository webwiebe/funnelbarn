package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestIsBrandedAliasHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"f.acme.nl", true},
		{"f.shop.acme.co.uk", true},
		{"f.my-brand.io", true},
		{"funnelbarn.wiebe.xyz", false},
		{"f.localhost", false},
		{"f.", false},
		{"fb.acme.nl", false},
		{"f..acme.nl", false},
		{"f.-acme.nl", false},
		{"f.acme-.nl", false},
		{"f.acme.nl/evil", false},
		{"f.acme.nl`)<script>", false},
		{"f.ACME.nl", false}, // callers lowercase first
		{"f." + strings.Repeat("a", 64) + ".nl", false},
	}
	for _, tt := range tests {
		if got := isBrandedAliasHost(tt.host); got != tt.want {
			t.Errorf("isBrandedAliasHost(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestSetupBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		proxies []string
		remote  string
		host    string
		fwdHost string
		want    string
	}{
		{name: "canonical host keeps public url", host: "funnelbarn.wiebe.xyz", want: "https://funnelbarn.wiebe.xyz"},
		{name: "branded alias is handed back", host: "f.acme.nl", want: "https://f.acme.nl"},
		{name: "alias port and case are normalised", host: "F.Acme.NL:443", want: "https://f.acme.nl"},
		{name: "forwarded host wins without proxy list", host: "funnelbarn:8080", fwdHost: "f.acme.nl, other", want: "https://f.acme.nl"},
		{name: "forwarded host from trusted proxy", proxies: []string{"10.0.0.1"}, remote: "10.0.0.1:1234", host: "funnelbarn:8080", fwdHost: "f.acme.nl", want: "https://f.acme.nl"},
		{name: "forwarded host from untrusted peer ignored", proxies: []string{"10.0.0.1"}, remote: "192.0.2.9:1234", host: "funnelbarn.wiebe.xyz", fwdHost: "f.evil.example", want: "https://funnelbarn.wiebe.xyz"},
		{name: "malformed alias falls back", host: "f.acme..nl", want: "https://funnelbarn.wiebe.xyz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &Server{publicURL: "https://funnelbarn.wiebe.xyz", trustedProxies: tt.proxies}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/x", nil)
			req.Host = tt.host
			if tt.remote != "" {
				req.RemoteAddr = tt.remote
			}
			if tt.fwdHost != "" {
				req.Header.Set("X-Forwarded-Host", tt.fwdHost)
			}
			if got := srv.setupBaseURL(req); got != tt.want {
				t.Errorf("setupBaseURL = %q, want %q", got, tt.want)
			}
		})
	}
}

// A customer onboarding through f.<brand> must be handed f.<brand> in every
// snippet, and never the canonical host they did not ask for.
func TestSetupDoc_BrandedAliasKeepsBrand(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/acme", nil)
	req.Host = "f.acme.nl"
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("setup doc on alias: want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		"| Endpoint   | https://f.acme.nl |",
		"| Setup URL  | https://f.acme.nl/api/v1/setup/acme |",
		"https://f.acme.nl/sdk.js",
		"https://f.acme.nl/api/v1/events",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("setup doc on alias missing %q", want)
		}
	}
	// newTestServer's PublicURL is http://localhost.
	if strings.Contains(body, "http://localhost") {
		t.Error("setup doc on alias still contains the canonical public URL")
	}
}

// iambarn fetches the theme manifest from the login origin and treats a 3xx as
// a failure, so the f.<domain> redirect must pass it through.
func TestThemeManifest_ServedOnBrandedHost(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, themeManifestPath, nil)
	req.Host = "f.bananasketch.nl"
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("theme manifest on f. host: want 200, got %d (Location %q)", w.Code, w.Header().Get("Location"))
	}
}

func TestHandleCreateProject_PersistsDomain(t *testing.T) {
	srv, store := newTestServer(t)

	w := postJSON(t, srv, "/api/v1/projects", map[string]string{
		"name":   "Acme",
		"domain": " acme.nl ",
	}, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("create project: want 201, got %d (body: %s)", w.Code, w.Body.String())
	}
	var p repository.Project
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Domain != "acme.nl" {
		t.Errorf("response Domain = %q, want acme.nl", p.Domain)
	}
	stored, err := store.ProjectByID(t.Context(), p.ID)
	if err != nil {
		t.Fatalf("ProjectByID: %v", err)
	}
	if stored.Domain != "acme.nl" || stored.Slug != "acme-nl" {
		t.Errorf("stored project = %+v, want domain acme.nl and slug acme-nl", stored)
	}
}
