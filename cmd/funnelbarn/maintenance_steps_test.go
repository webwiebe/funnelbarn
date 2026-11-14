package main

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/wiebe-xyz/funnelbarn/internal/config"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/repotest"
)

// scriptedRecordings answers the three sweeps with fixed results and records
// the retention it was given.
type scriptedRecordings struct {
	oldErr, brokenErr, botErr error
	broken, bot               int
	retention                 int
	calls                     []string
}

func (s *scriptedRecordings) PurgeOldRecordings(_ context.Context, days int) error {
	s.calls = append(s.calls, "old")
	s.retention = days
	return s.oldErr
}

func (s *scriptedRecordings) PurgeBrokenRecordings(_ context.Context) (int, error) {
	s.calls = append(s.calls, "broken")
	return s.broken, s.brokenErr
}

func (s *scriptedRecordings) PurgeBotRecordings(_ context.Context) (int, error) {
	s.calls = append(s.calls, "bot")
	return s.bot, s.botErr
}

func openMaintenanceStore(t *testing.T, name string) *repository.Store {
	t.Helper()
	store, err := repository.Open(repotest.Path(t, name))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func spansByName(exp *tracetest.InMemoryExporter) map[string]tracetest.SpanStub {
	out := map[string]tracetest.SpanStub{}
	for _, s := range exp.GetSpans() {
		out[s.Name] = s
	}
	return out
}

func intAttr(s tracetest.SpanStub, key string) (int64, bool) {
	for _, a := range s.Attributes {
		if string(a.Key) == key && a.Value.Type() == attribute.INT64 {
			return a.Value.AsInt64(), true
		}
	}
	return 0, false
}

// The steps run in a fixed order, so a slow pass reads the same way every time.
func TestRunMaintenance_StepOrder(t *testing.T) {
	exp := captureSpans(t)
	store := openMaintenanceStore(t, "maint-order.db")

	cfg := config.Config{EventRetentionDays: 30, AutoRegisterTTLDays: 30}
	runMaintenance(context.Background(), cfg, store, &scriptedRecordings{})

	var steps []tracetest.SpanStub
	for _, s := range exp.GetSpans() {
		if strings.HasPrefix(s.Name, "maintenance.purge.") {
			steps = append(steps, s)
		}
	}
	sort.Slice(steps, func(i, j int) bool { return steps[i].StartTime.Before(steps[j].StartTime) })
	var got []string
	for _, s := range steps {
		got = append(got, s.Name)
	}
	want := []string{
		"maintenance.purge.web_sessions",
		"maintenance.purge.events",
		"maintenance.purge.evaluations",
		"maintenance.purge.auto_flags",
		"maintenance.purge.orphans",
		"maintenance.purge.duplicate_ingest_ids",
		"maintenance.purge.recordings",
	}
	if len(got) != len(want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("steps = %v, want %v", got, want)
		}
	}
}

// Each retention step is gated on its own setting.
func TestRunMaintenance_RetentionStepsFollowConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		want map[string]bool
	}{
		{"events only", config.Config{EventRetentionDays: 7},
			map[string]bool{"events": true, "evaluations": true, "auto_flags": false}},
		{"auto flags only", config.Config{AutoRegisterTTLDays: 7},
			map[string]bool{"events": false, "evaluations": false, "auto_flags": true}},
		{"negative values disable", config.Config{EventRetentionDays: -1, AutoRegisterTTLDays: -1},
			map[string]bool{"events": false, "evaluations": false, "auto_flags": false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exp := captureSpans(t)
			store := openMaintenanceStore(t, "maint-gate.db")
			runMaintenance(context.Background(), tc.cfg, store, nil)
			spans := spansByName(exp)
			for step, wantRan := range tc.want {
				if _, ran := spans["maintenance.purge."+step]; ran != wantRan {
					t.Errorf("step %s ran = %v, want %v", step, ran, wantRan)
				}
			}
			for _, always := range []string{"web_sessions", "orphans", "duplicate_ingest_ids"} {
				if _, ok := spans["maintenance.purge."+always]; !ok {
					t.Errorf("step %s must always run", always)
				}
			}
		})
	}
}

// The recording retention comes from the instance setting when it is a
// positive integer, and is 90 days otherwise.
func TestRunMaintenance_RecordingRetentionDays(t *testing.T) {
	cases := []struct {
		name    string
		setting *string
		want    int
	}{
		{"unset", nil, 90},
		{"valid", ptr("30"), 30},
		{"not a number", ptr("abc"), 90},
		{"zero", ptr("0"), 90},
		{"negative", ptr("-5"), 90},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			captureSpans(t)
			store := openMaintenanceStore(t, "maint-retention.db")
			if tc.setting != nil {
				if err := store.SetInstanceSetting(context.Background(), "recording_retention_days", *tc.setting); err != nil {
					t.Fatalf("set setting: %v", err)
				}
			}
			rec := &scriptedRecordings{}
			runMaintenance(context.Background(), config.Config{}, store, rec)
			if rec.retention != tc.want {
				t.Errorf("retention = %d, want %d", rec.retention, tc.want)
			}
		})
	}
}

func ptr(s string) *string { return &s }

// The recording sweeps run in order, and a failing one does not stop the next.
// Every failure lands on the step span and on the root span.
func TestRunMaintenance_RecordingFailuresDoNotStopTheSweeps(t *testing.T) {
	exp := captureSpans(t)
	store := openMaintenanceStore(t, "maint-fail.db")
	rec := &scriptedRecordings{
		oldErr:    errors.New("old failed"),
		brokenErr: errors.New("broken failed"),
		botErr:    errors.New("bot failed"),
	}

	runMaintenance(context.Background(), config.Config{}, store, rec)

	if got := rec.calls; len(got) != 3 || got[0] != "old" || got[1] != "broken" || got[2] != "bot" {
		t.Fatalf("calls = %v, want [old broken bot]", got)
	}
	spans := spansByName(exp)
	step := spans["maintenance.purge.recordings"]
	if step.Status.Code != codes.Error {
		t.Errorf("recordings span status = %v, want Error", step.Status.Code)
	}
	if len(step.Events) != 3 {
		t.Errorf("recordings span has %d error events, want 3", len(step.Events))
	}
	root := spans["maintenance.purge"]
	if root.Status.Code != codes.Error {
		t.Errorf("root span status = %v, want Error", root.Status.Code)
	}
	if len(root.Events) != 3 {
		t.Errorf("root span has %d error events, want 3", len(root.Events))
	}
}

// Row counts from the broken and bot sweeps reach the step span, and a zero
// count leaves the attribute out.
func TestRunMaintenance_RecordingCountsAreSpanAttributes(t *testing.T) {
	exp := captureSpans(t)
	store := openMaintenanceStore(t, "maint-counts.db")
	runMaintenance(context.Background(), config.Config{}, store, &scriptedRecordings{broken: 2, bot: 3})

	step := spansByName(exp)["maintenance.purge.recordings"]
	if n, ok := intAttr(step, "broken_rows"); !ok || n != 2 {
		t.Errorf("broken_rows = %d (set %v), want 2", n, ok)
	}
	if n, ok := intAttr(step, "bot_rows"); !ok || n != 3 {
		t.Errorf("bot_rows = %d (set %v), want 3", n, ok)
	}
	if step.Status.Code == codes.Error {
		t.Error("a clean sweep must not mark the span as an error")
	}

	exp2 := captureSpans(t)
	runMaintenance(context.Background(), config.Config{}, store, &scriptedRecordings{})
	step = spansByName(exp2)["maintenance.purge.recordings"]
	if hasAttr(step, "broken_rows") || hasAttr(step, "bot_rows") {
		t.Error("zero counts must not set broken_rows or bot_rows")
	}
}

// A clean pass leaves the root span without an error status.
func TestRunMaintenance_CleanPassHasNoErrors(t *testing.T) {
	exp := captureSpans(t)
	store := openMaintenanceStore(t, "maint-clean.db")
	cfg := config.Config{EventRetentionDays: 30, AutoRegisterTTLDays: 30}
	runMaintenance(context.Background(), cfg, store, &scriptedRecordings{})

	root := spansByName(exp)["maintenance.purge"]
	if root.Status.Code == codes.Error || len(root.Events) != 0 {
		t.Errorf("root span status %v with %d events, want clean", root.Status.Code, len(root.Events))
	}
}
