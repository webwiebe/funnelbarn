package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

type fakeBackfiller struct {
	projects []domain.Project
	gotID    string
	gotApply bool
}

func (f *fakeBackfiller) ListProjects(context.Context) ([]domain.Project, error) {
	return f.projects, nil
}

func (f *fakeBackfiller) BackfillRecordingEnvironments(_ context.Context, id string, _ int, apply bool, onBatch func(repository.EnvironmentBackfill)) (repository.EnvironmentBackfill, error) {
	f.gotID, f.gotApply = id, apply
	if onBatch != nil {
		onBatch(repository.EnvironmentBackfill{Scanned: 5, Batches: 1})
	}
	return repository.EnvironmentBackfill{Scanned: 5, Filled: 3, Mixed: 1, NoEvents: 1, Batches: 1}, nil
}

func TestParseBackfillEnvFlags_RequiresAProject(t *testing.T) {
	_, err := parseBackfillEnvFlags(nil)
	assert.Error(t, err)
	f, err := parseBackfillEnvFlags([]string{"--project", "pensioenfeest", "--apply"})
	require.NoError(t, err)
	assert.True(t, f.apply)
	_, err = parseBackfillEnvFlags([]string{"--project", "x", "extra"})
	assert.Error(t, err)
}

func TestBackfillEnvironments_PrintsCountsForOneProject(t *testing.T) {
	st := &fakeBackfiller{projects: []domain.Project{{ID: "p1", Slug: "other"}, {ID: "p2", Slug: "pensioenfeest"}}}
	var out bytes.Buffer
	var prog bytes.Buffer
	require.NoError(t, backfillEnvironments(context.Background(), &out, &prog, st, stripFlags{project: "pensioenfeest"}))
	assert.Equal(t, "p2", st.gotID)
	assert.False(t, st.gotApply)
	assert.JSONEq(t, `{"project":"pensioenfeest","applied":false,"scanned":5,"filled":3,"mixed":1,"no_events":1,"batches":1}`, out.String())
	assert.Equal(t, "batch 1: scanned 5 filled 0 mixed 0 no_events 0\n", prog.String())
	assert.Error(t, backfillEnvironments(context.Background(), &out, &prog, st, stripFlags{project: "missing"}))
}
