package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
)

func scopedHandler(t *testing.T) *Handler {
	t.Helper()
	lookup := func(context.Context, string) (string, string, bool, error) { return "proj-1", "ingest", true, nil }
	h := NewHandler(auth.New("static").WithDBLookup(lookup, nil), newTestSpool(t), 0)
	h.ProjectSlug = func(_ context.Context, id string) (string, error) {
		if id != "proj-1" {
			return "", errors.New("unknown project")
		}
		return "professionals", nil
	}
	return h
}

func post(h *Handler, header string) map[string]any {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(`{"name":"pageview"}`))
	req.Header.Set(auth.HeaderAPIKey, "project-key")
	if header != "" {
		req.Header.Set("x-funnelbarn-project", header)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	body["_code"] = float64(w.Code)
	return body
}

func TestServeHTTP_AnswerNamesTheProject(t *testing.T) {
	h := scopedHandler(t)
	for _, header := range []string{"", "professionals", "proj-1"} {
		body := post(h, header)
		assert.Equal(t, float64(http.StatusAccepted), body["_code"], header)
		assert.Equal(t, "professionals", body["project"], header)
		assert.Equal(t, true, body["accepted"])
		assert.NotEmpty(t, body["ingestId"])
		assert.NotContains(t, body, "warning", "header %q matches the key's project", header)
	}
}

func TestServeHTTP_MismatchedProjectHeaderWarnsAndKeyWins(t *testing.T) {
	h := scopedHandler(t)
	body := post(h, "profotograaf")
	assert.Equal(t, float64(http.StatusAccepted), body["_code"])
	assert.Equal(t, "professionals", body["project"])
	assert.Contains(t, body["warning"], "profotograaf")
	assert.Contains(t, body["warning"], "professionals")
	rec := <-h.queue
	assert.Equal(t, "proj-1", rec.ProjectSlug, "the project of the key still files the event")
}

func TestServeHTTP_AnswerWithoutSlugLookupStaysValid(t *testing.T) {
	h := scopedHandler(t)
	h.ProjectSlug = nil
	body := post(h, "profotograaf")
	assert.Equal(t, float64(http.StatusAccepted), body["_code"])
	assert.NotContains(t, body, "project")
	assert.Contains(t, body["warning"], "profotograaf")
}

func TestServeHTTP_InstanceWideKeyAnswersWithTheHeaderProject(t *testing.T) {
	h := NewHandler(auth.New("key"), newTestSpool(t), 0)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(`{"name":"pageview"}`))
	req.Header.Set(auth.HeaderAPIKey, "key")
	req.Header.Set("x-funnelbarn-project", "my-site")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "my-site", body["project"])
	assert.NotContains(t, body, "warning")
}

func TestCheckProjectHeader_LogsOncePerProjectAndHeader(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := scopedHandler(t)
	for i := 0; i < 3; i++ {
		assert.NotEmpty(t, h.CheckProjectHeader(context.Background(), "proj-1", "professionals", "profotograaf"))
	}
	assert.NotEmpty(t, h.CheckProjectHeader(context.Background(), "proj-1", "professionals", "another"))
	assert.Equal(t, 2, strings.Count(buf.String(), `"level":"WARN"`), buf.String())
	assert.NotContains(t, buf.String(), `"level":"ERROR"`)
}

func TestCheckProjectHeader_CapsTheEchoedHeader(t *testing.T) {
	h := scopedHandler(t)
	w := h.CheckProjectHeader(context.Background(), "proj-1", "professionals", strings.Repeat("x", 500))
	assert.Less(t, len(w), 300)
}
