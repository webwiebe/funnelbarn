package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetricsToken(t *testing.T) {
	const token = "metrics-test-token"

	tests := []struct {
		name     string
		token    string // set on the server; empty leaves /metrics open
		auth     string // Authorization header; empty sends none
		wantCode int
	}{
		{name: "no token configured is open", wantCode: http.StatusOK},
		{name: "token set, no header", token: token, wantCode: http.StatusUnauthorized},
		{name: "token set, wrong token", token: token, auth: "Bearer other", wantCode: http.StatusUnauthorized},
		{name: "token set, no Bearer scheme", token: token, auth: token, wantCode: http.StatusUnauthorized},
		{name: "token set, right token", token: token, auth: "Bearer " + token, wantCode: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := newTestServer(t)
			if tt.token != "" {
				srv.SetMetricsToken(tt.token)
			}
			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("got status %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}
