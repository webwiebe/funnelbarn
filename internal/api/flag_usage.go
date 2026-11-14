package api

import (
	"net/http"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// registerFlagInsightRoutes serves the read-only flag insight endpoints. Both
// literal paths rank above /flags/{fid} in the mux.
func (s *Server) registerFlagInsightRoutes() {
	s.mux.HandleFunc("GET /api/v1/projects/{id}/flags/context-keys", s.requireSession(s.handleFlagContextKeys))
	s.mux.HandleFunc("GET /api/v1/projects/{id}/flags/usage", s.requireSessionOrToken(domain.APIKeyScopeFlagsRead, s.handleFlagUsage))
}

// handleFlagUsage reports per flag the recorded evaluations of the last 30
// UTC days, with the counts_note and stale_note that say what the numbers and
// the stale marker mean.
func (s *Server) handleFlagUsage(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		jsonError(w, "project id required", http.StatusBadRequest)
		return
	}
	report, err := s.flags.Usage(r.Context(), projectID)
	if err != nil {
		mapServiceError(w, err, "handleFlagUsage")
		return
	}
	writeJSON(w, http.StatusOK, report)
}
