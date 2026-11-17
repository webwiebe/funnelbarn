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

type fakeStripper struct {
	projects []domain.Project
	gotID    string
	gotApply bool
}

func (f *fakeStripper) ListProjects(context.Context) ([]domain.Project, error) {
	return f.projects, nil
}

func (f *fakeStripper) StripRecordingPageURLs(_ context.Context, id string, _ int, apply bool) (repository.PageURLStrip, error) {
	f.gotID, f.gotApply = id, apply
	return repository.PageURLStrip{Scanned: 4, Changed: 3, Batches: 1}, nil
}

func TestParseStripFlags_RequiresAProject(t *testing.T) {
	_, err := parseStripFlags(nil)
	assert.Error(t, err)
	f, err := parseStripFlags([]string{"--project", "iambarn", "--apply"})
	require.NoError(t, err)
	assert.True(t, f.apply)
	assert.Equal(t, "iambarn", f.project)
	_, err = parseStripFlags([]string{"--project", "iambarn", "extra"})
	assert.Error(t, err)
}

func TestStripPageURLs_PrintsCountsForOneProject(t *testing.T) {
	st := &fakeStripper{projects: []domain.Project{{ID: "p1", Slug: "other"}, {ID: "p2", Slug: "iambarn"}}}
	var out bytes.Buffer
	require.NoError(t, stripPageURLs(context.Background(), &out, st, stripFlags{project: "iambarn"}))
	assert.Equal(t, "p2", st.gotID)
	assert.False(t, st.gotApply, "no --apply is a dry run")
	assert.JSONEq(t, `{"project":"iambarn","applied":false,"scanned":4,"changed":3,"batches":1}`, out.String())

	err := stripPageURLs(context.Background(), &out, st, stripFlags{project: "missing"})
	assert.Error(t, err)
}
