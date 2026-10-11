package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

func seedAgentPR(t *testing.T, env *testEnv, projectID string, opened time.Time) {
	t.Helper()
	ctx := context.Background()
	for _, e := range []struct {
		name string
		at   time.Time
	}{{"pr_opened", opened}, {"pr_merged", opened.Add(90 * time.Minute)}} {
		if err := env.Store.InsertEvent(ctx, repository.Event{
			ID: e.name + projectID[:8], ProjectID: projectID, Name: e.name, IngestID: e.name + projectID[:8],
			OccurredAt: e.at, SessionID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Properties: `{"source":"github","repo":"o/r","pr":7,"branch":"issue-1/x","merge_sha":"abc"}`,
		}); err != nil {
			t.Fatalf("InsertEvent: %v", err)
		}
	}
}

func TestAgentsLeadTimesTool(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Agents", "agents")
	seedAgentPR(t, env, a.ID, time.Date(2026, time.October, 6, 9, 0, 0, 0, time.UTC))
	cs := env.connect(t, tokenRead, a.Slug)

	var out agentsLeadTimesOut
	callOK(t, cs, "agents_lead_times", map[string]any{"from": "2026-10-01", "to": "2026-10-11", "repo": "o/r"}, &out)
	if out.Project != a.Slug {
		t.Errorf("project = %q, want %q", out.Project, a.Slug)
	}
	if out.LeadTimes.PRToMerge.Count != 1 || out.LeadTimes.PRToMerge.MedianSeconds != 5400 {
		t.Errorf("pr_to_merge = %+v, want one PR merged after 5400s", out.LeadTimes.PRToMerge)
	}

	// Without from the window reaches back 30 days from to.
	callOK(t, cs, "agents_lead_times", map[string]any{"to": "2026-11-01"}, &out)
	if got := out.LeadTimes.To.Sub(out.LeadTimes.From); got != agentsDefaultRange {
		t.Errorf("default window = %s, want %s", got, agentsDefaultRange)
	}
	if out.LeadTimes.PRToMerge.Count != 1 {
		t.Errorf("default window missed the PR: %+v", out.LeadTimes.PRToMerge)
	}
}

func TestAgentsScoreboardTool(t *testing.T) {
	env := newTestEnv(t, nil)
	a := env.createProject(t, "Agents", "agents")
	cs := env.connect(t, tokenRead, a.Slug)

	var out agentsScoreboardOut
	callOK(t, cs, "agents_scoreboard", map[string]any{"week": "2026-W41"}, &out)
	if out.Week != "2026-W41" || out.Project != a.Slug {
		t.Errorf("got week %q project %q", out.Week, out.Project)
	}
	if want := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC); !out.Scoreboard.From.Equal(want) {
		t.Errorf("from = %s, want %s", out.Scoreboard.From, want)
	}

	callOK(t, cs, "agents_scoreboard", nil, &out)
	if !strings.HasPrefix(out.Week, "20") || !strings.Contains(out.Week, "-W") {
		t.Errorf("default week = %q, want the current ISO week", out.Week)
	}

	if msg := callErr(t, cs, "agents_scoreboard", map[string]any{"week": "2026-W99"}); !strings.Contains(msg, "week") {
		t.Errorf("bad week error = %q, want it to name the week", msg)
	}
}

func TestAgentTools_ErrorWithoutTheService(t *testing.T) {
	env := newTestEnv(t, func(d *Deps) { d.Agents = nil })
	a := env.createProject(t, "Agents", "agents")
	cs := env.connect(t, tokenRead, a.Slug)
	for _, name := range []string{"agents_scoreboard", "agents_lead_times"} {
		// A server wired without the service is a deployment fault, reported
		// like any unexpected failure (see get_setup_guide).
		if msg := callErr(t, cs, name, nil); msg != "internal error" {
			t.Errorf("%s: error = %q, want internal error", name, msg)
		}
	}
}

func TestAgentTools_ListedReadOnly(t *testing.T) {
	env := newTestEnv(t, nil)
	cs := env.connect(t, tokenRead, "")
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	seen := map[string]bool{"agents_scoreboard": false, "agents_lead_times": false}
	for _, tool := range res.Tools {
		if _, ok := seen[tool.Name]; !ok {
			continue
		}
		seen[tool.Name] = true
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s: ReadOnlyHint not set", tool.Name)
		}
	}
	for name, ok := range seen {
		if !ok {
			t.Errorf("%s not in tools/list", name)
		}
	}
}
