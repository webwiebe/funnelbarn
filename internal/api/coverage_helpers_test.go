package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// putRaw issues a PUT with a raw string body (used to send malformed JSON).
func putRaw(t *testing.T, srv *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

// postRaw issues a POST with a raw string body (used to send malformed JSON).
func postRaw(t *testing.T, srv *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

// fullServer builds an API server with auth disabled (open access, like
// newTestServer) but lets the caller wire up the optional services that
// newTestServer leaves nil (instance settings, geo anonymizer, segments,
// recording settings, OIDC, ...). The mutate hook runs on the ServerConfig
// before NewServer is called so tests can attach whatever they need.
func fullServer(t *testing.T, mutate func(cfg *ServerConfig)) (*Server, *repository.Store) {
	t.Helper()
	store := openMemoryStore(t)
	sp := newTestSpool(t)
	ingestHandler := ingest.NewHandler(auth.New("test-key"), sp, 0)
	sm := auth.NewSessionManager("test-secret", time.Hour)
	userAuth, _ := auth.NewUserAuthenticator("", "", "")

	cfg := ServerConfig{
		Ingest:              ingestHandler,
		Projects:            service.NewProjectService(store),
		Funnels:             service.NewFunnelService(store),
		ABTests:             service.NewABTestService(store),
		Flags:               service.NewFlagService(store),
		Events:              service.NewEventService(store),
		Overview:            service.NewOverviewService(store),
		Sessions:            service.NewSessionService(store),
		APIKeys:             service.NewAPIKeyService(store),
		Widgets:             service.NewWidgetService(store),
		Segments:            service.NewSegmentService(store),
		UserAuth:            userAuth,
		SessionManager:      sm,
		SessionSecret:       "test-secret",
		PublicURL:           "http://localhost",
		LoginRatePerMinute:  1000,
		LoginRateBurst:      1000,
		APIRatePerMinute:    1000,
		APIRateBurst:        1000,
		IngestRatePerMinute: 1000,
		IngestRateBurst:     1000,
		DB:                  store,
		Version:             "test",
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return NewServer(cfg), store
}
