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
			name:     "api path passes through on f-subdomain",
			host:     "f.pensioenfeest.nl",
			path:     "/api/v1/health",
			wantCode: http.StatusOK,
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
			req := httptest.NewRequest(http.MethodGet, "http://"+tt.host+tt.path, nil)
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
