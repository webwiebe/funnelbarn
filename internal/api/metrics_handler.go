package api

import (
	"crypto/subtle"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// SetMetricsToken configures a bearer token required to access /metrics.
// Call this after NewServer; an empty string means open access (backwards compatible).
func (s *Server) SetMetricsToken(token string) {
	s.metricsToken = token
	// Re-register the metrics route with the updated token.
	s.mux = http.NewServeMux()
	s.registerRoutes()
}

// metricsHandler returns the Prometheus metrics handler, optionally
// protected by a bearer token when metricsToken is non-empty.
func (s *Server) metricsHandler() http.Handler {
	promH := promhttp.Handler()
	if s.metricsToken == "" {
		return promH // no token configured — open access (backwards compatible)
	}
	expected := "Bearer " + s.metricsToken
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented := r.Header.Get("Authorization")
		// Constant-time compare to avoid leaking the token via response timing.
		if subtle.ConstantTimeCompare([]byte(presented), []byte(expected)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		promH.ServeHTTP(w, r)
	})
}
