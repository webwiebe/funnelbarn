package service

import (
	"bytes"
	"context"
	"database/sql"
	"runtime"
	"strconv"
	"sync"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/command"
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// goroutineID identifies the calling goroutine, so a fake store can tell a
// write made on the request goroutine from one made by the dispatcher.
func goroutineID() uint64 {
	buf := make([]byte, 64)
	buf = buf[:runtime.Stack(buf, false)]
	buf = bytes.TrimPrefix(buf, []byte("goroutine "))
	if i := bytes.IndexByte(buf, ' '); i > 0 {
		buf = buf[:i]
	}
	id, _ := strconv.ParseUint(string(buf), 10, 64)
	return id
}

// asyncFlagStore is a FlagRepo whose write methods fail the test when called
// on the request goroutine. Unimplemented methods panic through the nil
// embedded interface, which flags an unexpected call.
type asyncFlagStore struct {
	ports.FlagRepo
	t         *testing.T
	requestID uint64

	mu      sync.Mutex
	flags   map[string]repository.FeatureFlag
	records []repository.FlagEvaluation
	ensured []repository.FeatureFlag
	touched []string
}

func newAsyncFlagStore(t *testing.T) *asyncFlagStore {
	return &asyncFlagStore{t: t, requestID: goroutineID(), flags: map[string]repository.FeatureFlag{}}
}

func (s *asyncFlagStore) noSyncWrite(name string) {
	if goroutineID() == s.requestID {
		s.t.Errorf("%s ran on the request goroutine", name)
	}
}

func (s *asyncFlagStore) FlagByKey(_ context.Context, projectID, key string) (repository.FeatureFlag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.flags[projectID+"/"+key]
	if !ok {
		return repository.FeatureFlag{}, sql.ErrNoRows
	}
	return f, nil
}

func (s *asyncFlagStore) CountAutoFlags(context.Context, string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ensured), nil
}

func (s *asyncFlagStore) RecordEvaluation(_ context.Context, e repository.FlagEvaluation) error {
	s.noSyncWrite("RecordEvaluation")
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, e)
	return nil
}

func (s *asyncFlagStore) EnsureAutoFlag(_ context.Context, f repository.FeatureFlag) (repository.FeatureFlag, error) {
	s.noSyncWrite("EnsureAutoFlag")
	s.mu.Lock()
	defer s.mu.Unlock()
	f.ID = "auto-" + f.FlagKey
	s.flags[f.ProjectID+"/"+f.FlagKey] = f
	s.ensured = append(s.ensured, f)
	return f, nil
}

func (s *asyncFlagStore) TouchFlagEvaluated(_ context.Context, id string) error {
	s.noSyncWrite("TouchFlagEvaluated")
	s.mu.Lock()
	defer s.mu.Unlock()
	s.touched = append(s.touched, id)
	return nil
}

func startDispatcher(t *testing.T) *command.Dispatcher {
	t.Helper()
	d := command.New(command.Options{})
	d.Start(context.Background())
	t.Cleanup(func() {
		if err := d.Close(context.Background()); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return d
}

func TestEvaluateFlag_NoSynchronousWrite(t *testing.T) {
	store := newAsyncFlagStore(t)
	store.flags["p1/exp"] = repository.FeatureFlag{
		ID: "f1", ProjectID: "p1", FlagKey: "exp", Status: "active", Kind: repository.FlagKindExperiment,
		Variants: `{"on":true,"off":false}`, DefaultVariant: "off", Split: `{"on":100,"off":0}`, TargetingRules: "[]",
	}
	d := startDispatcher(t)
	svc := NewFlagService(store).WithCommands(d)

	res, err := svc.EvaluateFlag(context.Background(), "p1", "exp", map[string]any{"targetingKey": "u1"})
	if err != nil {
		t.Fatalf("EvaluateFlag: %v", err)
	}
	if res.Variant != "on" || res.Reason != "SPLIT" {
		t.Fatalf("unexpected result %+v", res)
	}
	if err := d.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(store.records) != 1 || store.records[0].FlagID != "f1" || store.records[0].Variant != "on" {
		t.Fatalf("want one evaluation row for f1/on after Flush, got %+v", store.records)
	}
}

func TestEvaluateFlag_PreviewWritesNothing(t *testing.T) {
	store := newAsyncFlagStore(t)
	store.flags["p1/exp"] = repository.FeatureFlag{
		ID: "f1", ProjectID: "p1", FlagKey: "exp", Status: "active", Kind: repository.FlagKindExperiment,
		Variants: `{"on":true}`, DefaultVariant: "on", Split: `{"on":100}`, TargetingRules: "[]",
	}
	d := startDispatcher(t)
	svc := NewFlagService(store).WithCommands(d)
	if _, err := svc.PreviewFlag(context.Background(), "p1", "exp", nil); err != nil {
		t.Fatal(err)
	}
	if err := d.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.records) != 0 {
		t.Fatalf("preview recorded %d rows", len(store.records))
	}
}

func TestEvaluateOrRegisterFlag_AsyncReturnsDisabledDefault(t *testing.T) {
	store := newAsyncFlagStore(t)
	d := startDispatcher(t)
	svc := NewFlagService(store).WithCommands(d)
	ctx, wait := WithSubmitWait(context.Background())

	res, err := svc.EvaluateOrRegisterFlag(ctx, "p1", "new_flag", nil, true, 5, "")
	if err != nil {
		t.Fatalf("EvaluateOrRegisterFlag: %v", err)
	}
	if res.Value != true || res.Variant != "default" || res.Reason != "DISABLED" || res.FlagKey != "new_flag" {
		t.Fatalf("want DISABLED default, got %+v", res)
	}
	if wait.Total() < 0 {
		t.Fatal("negative submit wait")
	}
	if err := d.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.ensured) != 1 || store.ensured[0].FlagKey != "new_flag" || store.ensured[0].Origin != "auto" {
		t.Fatalf("want one auto flag after Flush, got %+v", store.ensured)
	}
	if len(store.touched) != 1 || store.touched[0] != "auto-new_flag" {
		t.Fatalf("want a touch of the new flag after it exists, got %+v", store.touched)
	}

	// A second evaluation now finds the flag, which is inactive: same answer.
	res, err = svc.EvaluateOrRegisterFlag(ctx, "p1", "new_flag", nil, true, 5, "")
	if err != nil || res.Reason != "DISABLED" || res.Value != true {
		t.Fatalf("second evaluation: %+v, %v", res, err)
	}
}

func TestEvaluateOrRegisterFlag_AsyncRespectsCap(t *testing.T) {
	store := newAsyncFlagStore(t)
	store.ensured = []repository.FeatureFlag{{}, {}}
	d := startDispatcher(t)
	svc := NewFlagService(store).WithCommands(d)
	if _, err := svc.EvaluateOrRegisterFlag(context.Background(), "p1", "x", nil, 1, 2, ""); err == nil {
		t.Fatal("want the auto-register limit error")
	}
	if err := d.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.ensured) != 2 {
		t.Fatalf("a capped project must not gain a flag, have %d", len(store.ensured))
	}
}
