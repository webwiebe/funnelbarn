package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// failingKeyIssuer stands in for the key service when the random source fails.
type failingKeyIssuer struct{ service.APIKeys }

func (failingKeyIssuer) IssueAPIKey(context.Context, string, string, string) (domain.APIKey, string, error) {
	return domain.APIKey{}, "", &service.KeyGenerationError{Err: errors.New("entropy exhausted")}
}

// A random-source failure answers 500 "failed to generate key", not a mapped
// service error.
func TestHandleCreateAPIKey_RandomSourceFailureIs500(t *testing.T) {
	srv, store := fullServer(t, func(cfg *ServerConfig) {
		cfg.APIKeys = failingKeyIssuer{cfg.APIKeys}
	})
	p, err := store.CreateProject(context.Background(), "Rand", "rand")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	w := postJSON(t, srv, "/api/v1/apikeys", map[string]string{"project_id": p.ID, "name": "k"}, nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d (body: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "failed to generate key") {
		t.Errorf("body %q lacks the generate-key message", w.Body.String())
	}
}
