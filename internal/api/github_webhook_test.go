package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
	"github.com/wiebe-xyz/funnelbarn/internal/enrich"
	"github.com/wiebe-xyz/funnelbarn/internal/ghwebhook"
	"github.com/wiebe-xyz/funnelbarn/internal/ingest"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
	"github.com/wiebe-xyz/funnelbarn/internal/spool"
	"github.com/wiebe-xyz/funnelbarn/internal/worker"
)

const testWebhookSecret = "webhook-test-secret"

type webhookStack struct {
	srv     *Server
	store   *repository.Store
	sp      *spool.Spool
	handler *ingest.Handler
}

func newWebhookStack(t *testing.T, secret string) *webhookStack {
	t.Helper()
	store := openMemoryStore(t)
	sp := newTestSpool(t)
	handler := ingest.NewHandler(auth.New("test-key"), sp, 0)
	userAuth, _ := auth.NewUserAuthenticator("", "", "")
	srv := NewServer(ServerConfig{
		Ingest:              handler,
		Projects:            service.NewProjectService(store),
		Events:              service.NewEventService(store),
		UserAuth:            userAuth,
		SessionManager:      auth.NewSessionManager("test-secret", time.Hour),
		WebSessions:         store,
		SessionSecret:       "test-secret",
		PublicURL:           "http://localhost",
		IngestRatePerMinute: 1000,
		IngestRateBurst:     1000,
		DB:                  store,
		Version:             "test",
	})
	srv.SetGitHubWebhook(secret, "agents")
	return &webhookStack{srv: srv, store: store, sp: sp, handler: handler}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "ghwebhook", "testdata", name+".json"))
	require.NoError(t, err)
	return b
}

// deliver posts body as one GitHub delivery, signed with secret unless
// signature is set.
func (s *webhookStack) deliver(t *testing.T, kind, delivery string, body []byte, signature string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, githubWebhookPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", kind)
	if delivery != "" {
		req.Header.Set("X-GitHub-Delivery", delivery)
	}
	if signature != "" {
		req.Header.Set("X-Hub-Signature-256", signature)
	}
	w := httptest.NewRecorder()
	s.srv.ServeHTTP(w, req)
	return w
}

// persistSpool waits for want records to reach the spool and stores them the
// way the worker loop does.
func (s *webhookStack) persistSpool(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(asyncTimeout)
	var recs []spool.Record
	for {
		var err error
		recs, err = spool.ReadRecords(s.sp.Path())
		if err == nil && len(recs) == want {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("spool did not reach %d records (got %d, err=%v)", want, len(recs), err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	for _, rec := range recs {
		ev, err := worker.ProcessRecord(rec)
		require.NoError(t, err)
		proj, err := s.store.EnsureProject(context.Background(), rec.ProjectSlug)
		require.NoError(t, err)
		ev.ProjectID = proj.ID
		require.NoError(t, worker.PersistEvent(context.Background(), s.store, ev, nil))
	}
}

type storedEvent struct {
	name, userIDHash, ingestID, environment string
	props                                   map[string]any
}

func (s *webhookStack) agentsEvents(t *testing.T) []storedEvent {
	t.Helper()
	proj, err := s.store.EnsureProject(context.Background(), "agents")
	require.NoError(t, err)
	rows, err := s.store.ReadDB().QueryContext(context.Background(),
		`SELECT name, COALESCE(user_id_hash, ''), ingest_id, environment, COALESCE(properties, '{}') FROM events WHERE project_id = ? ORDER BY occurred_at`, proj.ID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out []storedEvent
	for rows.Next() {
		var e storedEvent
		var props string
		require.NoError(t, rows.Scan(&e.name, &e.userIDHash, &e.ingestID, &e.environment, &props))
		require.NoError(t, json.Unmarshal([]byte(props), &e.props))
		out = append(out, e)
	}
	require.NoError(t, rows.Err())
	return out
}

func TestGitHubWebhook_StoresEventsInTheAgentsProject(t *testing.T) {
	s := newWebhookStack(t, testWebhookSecret)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.handler.Start(ctx) }()

	deliveries := []struct{ kind, fixture, delivery string }{
		{"issues", "issues_opened", "d-issue"},
		{"pull_request", "pull_request_opened", "d-pr"},
		{"pull_request", "pull_request_closed_merged", "d-merged"},
		{"pull_request", "pull_request_closed_merged", "d-merged"}, // GitHub redelivery
		{"ping", "ping", "d-ping"},
	}
	for _, d := range deliveries {
		body := readFixture(t, d.fixture)
		w := s.deliver(t, d.kind, d.delivery, body, ghwebhook.Sign(testWebhookSecret, body))
		require.Equal(t, http.StatusAccepted, w.Code, "%s: %s", d.fixture, w.Body.String())
	}
	s.persistSpool(t, 4) // the ping is acknowledged, not spooled
	cancel()
	<-done

	events := s.agentsEvents(t)
	require.Len(t, events, 3, "the redelivered merge is stored once")
	issueHash := enrich.HashUserID("webwiebe/funnelbarn#331")
	for i, want := range []string{"issue_opened", "pr_opened", "pr_merged"} {
		require.Equal(t, want, events[i].name)
		require.Equal(t, issueHash, events[i].userIDHash, "%s is linked to issue #331", want)
		require.Equal(t, "webwiebe/funnelbarn#331", events[i].props["issue_ref"])
		require.Equal(t, "github", events[i].props["source"])
		require.Equal(t, "production", events[i].environment)
	}
	require.Equal(t, "d-merged", events[2].ingestID)
}

func TestGitHubWebhook_RejectsUnsignedAndForgedDeliveries(t *testing.T) {
	s := newWebhookStack(t, testWebhookSecret)
	body := readFixture(t, "issues_opened")

	w := s.deliver(t, "issues", "d1", body, "")
	require.Equal(t, http.StatusUnauthorized, w.Code, "missing signature")

	w = s.deliver(t, "issues", "d2", body, ghwebhook.Sign("not-the-secret", body))
	require.Equal(t, http.StatusUnauthorized, w.Code, "wrong secret")

	tampered := bytes.Replace(body, []byte(`"opened"`), []byte(`"closed"`), 1)
	w = s.deliver(t, "issues", "d3", tampered, ghwebhook.Sign(testWebhookSecret, body))
	require.Equal(t, http.StatusUnauthorized, w.Code, "body changed after signing")
}

func TestGitHubWebhook_BadRequests(t *testing.T) {
	s := newWebhookStack(t, testWebhookSecret)

	body := readFixture(t, "issues_opened")
	w := s.deliver(t, "issues", "", body, ghwebhook.Sign(testWebhookSecret, body))
	require.Equal(t, http.StatusBadRequest, w.Code, "missing X-GitHub-Delivery")

	junk := []byte("{not json")
	w = s.deliver(t, "issues", "d-junk", junk, ghwebhook.Sign(testWebhookSecret, junk))
	require.Equal(t, http.StatusBadRequest, w.Code, "undecodable payload")

	big := []byte(`{"pad":"` + strings.Repeat("x", githubWebhookMaxBody) + `"}`)
	w = s.deliver(t, "issues", "d-big", big, ghwebhook.Sign(testWebhookSecret, big))
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code, "over the payload cap")
}

func TestGitHubWebhook_NotFoundWithoutSecret(t *testing.T) {
	s := newWebhookStack(t, "")
	body := readFixture(t, "issues_opened")
	w := s.deliver(t, "issues", "d1", body, ghwebhook.Sign("", body))
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestGitHubWebhook_ReaderServesItLocally(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, githubWebhookPath, nil)
	require.False(t, forwardsToWriter(req), "a reader enqueues deliveries itself")
}
