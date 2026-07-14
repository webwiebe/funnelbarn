package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFDomainRedirect(t *testing.T) {
	srv, _ := newTestServer(t)

	tests := []struct {
		name     string
		method   string // defaults to GET when empty
		host     string
		path     string
		wantCode int
		wantLoc  string
	}{
		{
			name:     "redirects root to parent domain",
			host:     "f.pensioenfeest.nl",
			path:     "/",
			wantCode: http.StatusMovedPermanently,
			wantLoc:  "https://pensioenfeest.nl",
		},
		{
			name:     "redirects other f-subdomains",
			host:     "f.scanoo.nl",
			path:     "/",
			wantCode: http.StatusMovedPermanently,
			wantLoc:  "https://scanoo.nl",
		},
		{
			// curl -sI sends HEAD — the acceptance check for the bare-host
			// redirect. It must 301 just like GET, not fall through to 404.
			name:     "HEAD on root redirects like GET",
			method:   http.MethodHead,
			host:     "f.profotograaf.nl",
			path:     "/",
			wantCode: http.StatusMovedPermanently,
			wantLoc:  "https://profotograaf.nl",
		},
		{
			// Any non-ingest path, not just the exact root, redirects to the app.
			name:     "non-root non-ingest path redirects to app root",
			host:     "f.profotograaf.nl",
			path:     "/pricing",
			wantCode: http.StatusMovedPermanently,
			wantLoc:  "https://profotograaf.nl",
		},
		{
			name:     "api path passes through on f-subdomain",
			host:     "f.pensioenfeest.nl",
			path:     "/api/v1/health",
			wantCode: http.StatusOK,
		},
		{
			// SDK bundle is a pass-through path (served by nginx at the edge),
			// so it must not be swallowed by the redirect.
			name:     "sdk path is not redirected",
			host:     "f.pensioenfeest.nl",
			path:     "/sdk.js",
			wantCode: http.StatusNotFound, // Go service has no /sdk.js route; edge routes it to nginx
		},
		{
			name:     "non-f host root is not redirected",
			host:     "funnelbarn.wiebe.xyz",
			path:     "/",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "host with port strips port before check",
			host:     "f.example.com:8080",
			path:     "/",
			wantCode: http.StatusMovedPermanently,
			wantLoc:  "https://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := tt.method
			if method == "" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, "http://"+tt.host+tt.path, nil)
			req.Host = tt.host
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("got status %d, want %d", w.Code, tt.wantCode)
			}
			if tt.wantLoc != "" {
				loc := w.Header().Get("Location")
				if loc != tt.wantLoc {
					t.Errorf("Location: got %q, want %q", loc, tt.wantLoc)
				}
			}
		})
	}
}
