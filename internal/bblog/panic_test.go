package bblog_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/bblog"
)

func TestReportPanic_PostsToBugBarn(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/events" {
			received = make([]byte, r.ContentLength)
			r.Body.Read(received)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	bblog.ReportPanic(srv.URL, "test-key", "test panic value")

	if len(received) == 0 {
		t.Fatal("expected POST to BugBarn, got nothing")
	}
	body := string(received)
	if !strings.Contains(body, "test panic value") {
		t.Errorf("panic value not in body: %s", body)
	}
	if !strings.Contains(body, "panic") {
		t.Errorf("event type not in body: %s", body)
	}
}

func TestReportPanic_NoopWhenUnconfigured(t *testing.T) {
	// Should not panic or error when endpoint/key are empty
	bblog.ReportPanic("", "", "some panic")
	bblog.ReportPanic("http://localhost", "", "some panic")
}

func TestReportPanic_HandlesInvalidURL(t *testing.T) {
	// Should not panic when endpoint is invalid
	bblog.ReportPanic("not-a-url", "key", "panic value")
}
