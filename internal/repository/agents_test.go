package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func TestAgentEvents_FiltersNamesAndRangeOldestFirst(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	p, err := s.CreateProject(ctx, "Agents", "agents")
	require.NoError(t, err)
	other, err := s.CreateProject(ctx, "Other", "other")
	require.NoError(t, err)

	base := time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC)
	insert := func(project, name string, at time.Time, props string) {
		t.Helper()
		require.NoError(t, s.InsertEvent(ctx, repository.Event{
			ID: fmt.Sprintf("%s-%d", name, at.Unix()) + project[:4], ProjectID: project, SessionID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			UserIDHash: "hash-" + name, Name: name, Properties: props,
			IngestID: fmt.Sprintf("i-%s-%d-%s", name, at.Unix(), project[:4]), OccurredAt: at,
		}))
	}
	insert(p.ID, "pr_merged", base.Add(2*time.Hour), `{"source":"github","pr":12}`)
	insert(p.ID, "issue_opened", base, `{"source":"github","issue_ref":"o/r#1"}`)
	insert(p.ID, "subagent_finished", base.Add(time.Hour), `{}`)   // name not asked for
	insert(p.ID, "issue_opened", base.Add(-48*time.Hour), `{}`)    // before the range
	insert(other.ID, "issue_opened", base.Add(time.Hour), `{}`)    // other project
	insert(p.ID, "commit_made", base.Add(3*time.Hour), `not json`) // undecodable properties

	got, err := s.AgentEvents(ctx, p.ID, []string{"issue_opened", "pr_merged", "commit_made"}, base.Add(-time.Hour), base.Add(4*time.Hour))
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "issue_opened", got[0].Name)
	assert.True(t, got[0].OccurredAt.Equal(base))
	assert.Equal(t, "o/r#1", got[0].Properties["issue_ref"])
	assert.Equal(t, "hash-issue_opened", got[0].UserIDHash)
	assert.Equal(t, "pr_merged", got[1].Name)
	assert.Equal(t, float64(12), got[1].Properties["pr"])
	assert.Equal(t, "commit_made", got[2].Name)
	assert.Empty(t, got[2].Properties)

	none, err := s.AgentEvents(ctx, p.ID, nil, base, base.Add(time.Hour))
	require.NoError(t, err)
	assert.Empty(t, none)
}
