package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ctxRecordingPinger reports whether the context handed to Ping was already
// cancelled, which is how a prober's timeout used to reach the database call.
type ctxRecordingPinger struct {
	err        error // returned regardless of context state
	sawErr     error // ctx.Err() observed at call time
	hadDeadlin bool
	deadline   time.Time
	calls      int
}

func (p *ctxRecordingPinger) Ping(ctx context.Context) error {
	p.calls++
	p.sawErr = ctx.Err()
	p.deadline, p.hadDeadlin = ctx.Deadline()
	if p.err != nil {
		return p.err
	}
	// A ping that only fails when its context is done — the shape of a real
	// driver call that is interrupted rather than broken.
	return ctx.Err()
}

// cancelledRequest returns a health request whose context is already cancelled,
// standing in for kube-probe hanging up at timeoutSeconds.
func cancelledRequest() *http.Request {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return httptest.NewRequest(http.MethodGet, "/api/v1/health", nil).WithContext(ctx)
}

// kube-probe defaults to timeoutSeconds: 1 and the manifests did not set it, so
// a ping slower than a second was cancelled by the prober while the handler was
// still inside its own 2s budget. Ping returned context.Canceled, the handler
// logged that at Error, and bblog filed it as a BugBarn issue reading only
// "context canceled" (FUN-17), alongside the 503 envelope it caused (FUN-16).
// The prober's clock must not be able to fail our health check.
func TestHandleHealth_CallerCancellationDoesNotFailThePing(t *testing.T) {
	srv, _ := newTestServer(t)
	pinger := &ctxRecordingPinger{}
	srv.db = pinger

	w := httptest.NewRecorder()
	srv.handleHealth(w, cancelledRequest())

	if pinger.calls != 1 {
		t.Fatalf("want the ping attempted once, got %d calls", pinger.calls)
	}
	if pinger.sawErr != nil {
		t.Errorf("ping context was already cancelled (%v); it must be detached from the caller", pinger.sawErr)
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d — a cancelled prober must not mark the service unhealthy", w.Code, http.StatusOK)
	}
}

// The ping still runs under its own budget, so a genuinely wedged database
// cannot block the handler indefinitely.
func TestHandleHealth_PingCarriesItsOwnDeadline(t *testing.T) {
	srv, _ := newTestServer(t)
	pinger := &ctxRecordingPinger{}
	srv.db = pinger

	before := time.Now()
	srv.handleHealth(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	if !pinger.hadDeadlin {
		t.Fatal("ping context has no deadline")
	}
	if budget := pinger.deadline.Sub(before); budget > healthPingTimeout+time.Second || budget <= 0 {
		t.Errorf("ping budget = %v, want about %v", budget, healthPingTimeout)
	}
}

// Narrowing what counts as a failure must not hide a real one: a database that
// actually refuses the ping still reports 503.
func TestHandleHealth_RealPingFailureStillReportsUnhealthy(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.db = &ctxRecordingPinger{err: errors.New("connection refused")}

	w := httptest.NewRecorder()
	srv.handleHealth(w, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// A real failure is still reported even when the caller has gone, so a probe
// that times out against a broken database does not silently return 200.
func TestHandleHealth_RealPingFailureReportedEvenWhenCallerGone(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.db = &ctxRecordingPinger{err: errors.New("disk I/O error")}

	w := httptest.NewRecorder()
	srv.handleHealth(w, cancelledRequest())

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}
