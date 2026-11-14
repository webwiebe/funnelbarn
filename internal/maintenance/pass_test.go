package maintenance

import (
	"context"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/repotest"
)

// fakeRecordings records which sweeps ran, so the test can prove the
// maintenance pass reaches all three rather than just not crashing.
type fakeRecordings struct {
	oldCalled    bool
	brokenCalled bool
	botCalled    bool
	retentionGot int
}

func (f *fakeRecordings) PurgeOldRecordings(_ context.Context, retentionDays int) error {
	f.oldCalled = true
	f.retentionGot = retentionDays
	return nil
}

func (f *fakeRecordings) PurgeBrokenRecordings(_ context.Context) (int, error) {
	f.brokenCalled = true
	return 0, nil
}

func (f *fakeRecordings) PurgeBotRecordings(_ context.Context) (int, error) {
	f.botCalled = true
	return 0, nil
}

// The maintenance pass must reach every sweep. PurgeBotRecordings in particular
// is the only thing that can delete a bot recording's R2 chunks, so a pass that
// silently skips it leaves them stored forever.
func TestRunMaintenance_ReachesEverySweep(t *testing.T) {
	store, err := repository.Open(repotest.Path(t, "maint.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	rec := &fakeRecordings{}
	cfg := config.Config{EventRetentionDays: 30, AutoRegisterTTLDays: 30}

	Run(context.Background(), cfg, store, rec)

	if !rec.oldCalled {
		t.Error("PurgeOldRecordings was not called")
	}
	if !rec.brokenCalled {
		t.Error("PurgeBrokenRecordings was not called")
	}
	if !rec.botCalled {
		t.Error("PurgeBotRecordings was not called")
	}
	if rec.retentionGot != 90 {
		t.Errorf("retention days: want the 90-day default, got %d", rec.retentionGot)
	}
}

// Recordings are optional (no object storage configured), and the pass must
// still run the database-side work rather than skipping or panicking.
func TestRunMaintenance_NilRecordingsIsSafe(t *testing.T) {
	store, err := repository.Open(repotest.Path(t, "maint-nil.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	Run(context.Background(), config.Config{}, store, nil)
}

// The first pass must happen soon after boot, not a full day later. A redeploy
// resets the 24h ticker, and this instance redeploys more often than daily —
// with a startup delay of a day the pass would never run at all.
func TestStartupDelay_IsShortEnoughToSurviveRedeploys(t *testing.T) {
	if StartupDelay <= 0 {
		t.Fatalf("StartupDelay must be positive, got %v", StartupDelay)
	}
	if StartupDelay >= time.Hour {
		t.Errorf("StartupDelay is %v; a delay this long is reset by an ordinary redeploy, which is the bug this exists to fix", StartupDelay)
	}
}
