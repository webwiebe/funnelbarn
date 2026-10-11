package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestForwardsToWriter(t *testing.T) {
	cases := []struct {
		method, path string
		want         bool
	}{
		{"GET", "/api/v1/projects", false},
		{"HEAD", "/api/v1/projects", false},
		{"OPTIONS", "/api/v1/projects", false},
		{"POST", "/api/v1/projects", true},
		{"PUT", "/api/v1/projects/p1", true},
		{"DELETE", "/api/v1/projects/p1", true},
		{"PATCH", "/api/v1/projects/p1", true},
		{"POST", "/api/v1/login", true},
		{"POST", "/api/v1/oidc/backchannel-logout", true},
		{"POST", "/api/v1/projects/p1/flags/evaluate", true},
		{"POST", "/api/v1/events", false},
		{"POST", "/api/v1/recordings/chunk", false},
		{"POST", "/api/v1/evaluate", false},
		{"POST", "/api/v1/telemetry", false},
		{"POST", "/api/v1/client-errors", false},
		{"GET", "/api/v1/oidc/callback", true},
		{"GET", "/api/v1/auth/oidc/logged-out", true},
		{"GET", "/api/v1/oidc/login", false},
		{"GET", "/api/v1/setup/acme", true},
		{"GET", mcpPath, true},
		{"POST", mcpPath, true},
		{"DELETE", mcpPath, true},
		{"GET", mcpResourceMetaPath, false},
	}
	for _, c := range cases {
		if got := forwardsToWriter(httptest.NewRequest(c.method, c.path, nil)); got != c.want {
			t.Errorf("%s %s: forwardsToWriter = %v, want %v", c.method, c.path, got, c.want)
		}
	}
}

// fakeWriter records the requests it gets and answers with respond.
type fakeWriter struct {
	got     []*http.Request
	bodies  []string
	respond func(w http.ResponseWriter, r *http.Request)
}

func (f *fakeWriter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	f.got = append(f.got, r)
	f.bodies = append(f.bodies, string(b))
	if f.respond != nil {
		f.respond(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// newReaderServer is an authed server forwarding to writer.
func newReaderServer(t *testing.T, writer http.Handler) (*Server, *repository.Store) {
	t.Helper()
	srv, store := newAuthedServer(t)
	hs := httptest.NewServer(writer)
	t.Cleanup(hs.Close)
	srv.forwarder = newWriteForwarder(hs.URL, "internal-token")
	return srv, store
}

func TestForwardCarriesTheClientAndTheWritersAnswer(t *testing.T) {
	fw := &fakeWriter{respond: func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "funnelbarn_session", Value: "v"})
		w.Header().Set("Access-Control-Allow-Origin", "https://evil.example")
		w.Header().Set("Location", "/login")
		w.WriteHeader(http.StatusFound)
		_, _ = w.Write([]byte("moved"))
	}}
	srv, _ := newReaderServer(t, fw)
	srv.allowedOrigins = []string{"https://app.example"}

	req := httptest.NewRequest("POST", "/api/v1/login?next=%2Fprojects", strings.NewReader(`{"username":"admin"}`))
	req.Host = "funnelbarn.example"
	req.RemoteAddr = "203.0.113.7:5555"
	req.Header.Set("Origin", "https://app.example")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderInternalToken, "forged")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if len(fw.got) != 1 {
		t.Fatalf("writer got %d requests, want 1", len(fw.got))
	}
	up := fw.got[0]
	for header, want := range map[string]string{
		"X-Forwarded-For":   "203.0.113.7",
		"X-Forwarded-Proto": "http",
		"X-Forwarded-Host":  "funnelbarn.example",
		HeaderInternalToken: "internal-token",
		"Content-Type":      "application/json",
	} {
		if got := up.Header.Get(header); got != want {
			t.Errorf("forwarded %s = %q, want %q", header, got, want)
		}
	}
	if up.Host != "funnelbarn.example" || up.URL.RequestURI() != "/api/v1/login?next=%2Fprojects" {
		t.Errorf("forwarded to host %q uri %q", up.Host, up.URL.RequestURI())
	}
	if fw.bodies[0] != `{"username":"admin"}` {
		t.Errorf("forwarded body %q", fw.bodies[0])
	}

	// The redirect goes back to the browser, with the writer's cookie, and the
	// reader's own CORS decision.
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login" || rec.Body.String() != "moved" {
		t.Errorf("answer %d Location=%q body=%q", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), "funnelbarn_session=v") {
		t.Errorf("Set-Cookie = %q", rec.Header().Get("Set-Cookie"))
	}
	if got := rec.Header().Values("Access-Control-Allow-Origin"); len(got) != 1 || got[0] != "https://app.example" {
		t.Errorf("Access-Control-Allow-Origin = %v, want the reader's single value", got)
	}
}

func TestForwardAnswers502WhileTheWriterIsDown(t *testing.T) {
	srv, _ := newAuthedServer(t)
	hs := httptest.NewServer(http.NotFoundHandler())
	hs.Close()
	srv.forwarder = newWriteForwarder(hs.URL, "t")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/login", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want 502", rec.Code)
	}
}

func TestReaderServesReadsItself(t *testing.T) {
	fw := &fakeWriter{}
	srv, _ := newReaderServer(t, fw)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/health", nil))
	if rec.Code != http.StatusOK || len(fw.got) != 0 {
		t.Fatalf("status %d, writer got %d requests", rec.Code, len(fw.got))
	}
}

// A session past its absolute expiry is deleted when it is used, so a reader
// hands the request to the writer.
func TestReaderForwardsASessionThatNeedsAWrite(t *testing.T) {
	fw := &fakeWriter{respond: func(w http.ResponseWriter, _ *http.Request) {
		jsonError(w, "session expired", http.StatusUnauthorized)
	}}
	srv, store := newReaderServer(t, fw)

	live := sessionCookieFor(t, srv, "admin")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	req.AddCookie(live)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(fw.got) != 0 {
		t.Fatalf("live session: status %d, writer got %d requests", rec.Code, len(fw.got))
	}

	token, _, err := srv.sessionManager.Create()
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Minute).Unix()
	if err := store.CreateWebSession(context.Background(), repository.WebSession{
		IDHash: auth.HashSessionToken(token), Username: "admin", AuthMethod: "local",
		CreatedAt: past - 3600, AbsoluteExpiresAt: past,
	}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/v1/me", nil)
	req.AddCookie(auth.SessionCookie(token, time.Now().Add(time.Hour), false))
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || len(fw.got) != 1 {
		t.Fatalf("expired session: status %d, writer got %d requests", rec.Code, len(fw.got))
	}
	if _, err := store.GetWebSession(context.Background(), auth.HashSessionToken(token)); err != nil {
		t.Errorf("the reader deleted the session itself: %v", err)
	}
}

// The writer believes a forwarded client address only with the internal token.
func TestWriterTrustsForwardedHeadersWithTheToken(t *testing.T) {
	srv, _ := newAuthedServer(t)
	srv.trustedProxies = []string{"10.0.0.1"}
	srv.internalToken = "internal-token"

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.42.0.9:1234" // a reader pod, not a trusted proxy
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "funnelbarn.example")
	if got := srv.clientIP(req); got != "10.42.0.9" {
		t.Errorf("without the token clientIP = %q, want the peer", got)
	}

	req.Header.Set(HeaderInternalToken, "wrong")
	if got := srv.clientIP(req); got != "10.42.0.9" {
		t.Errorf("with a wrong token clientIP = %q, want the peer", got)
	}

	req.Header.Set(HeaderInternalToken, "internal-token")
	if got := srv.clientIP(req); got != "203.0.113.7" {
		t.Errorf("with the token clientIP = %q, want the forwarded client", got)
	}
	if !srv.isSecureRequest(req) || srv.requestHost(req) != "funnelbarn.example" {
		t.Errorf("with the token: secure=%v host=%q", srv.isSecureRequest(req), srv.requestHost(req))
	}

	srv.internalToken = ""
	if got := srv.clientIP(req); got != "10.42.0.9" {
		t.Errorf("a server without a token trusts a forwarded request: clientIP = %q", got)
	}
}
