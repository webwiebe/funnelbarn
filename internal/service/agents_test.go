package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/enrich"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

type fakeAgentQueries struct {
	events []repository.AgentEvent
	err    error
}

func (f fakeAgentQueries) AgentEvents(_ context.Context, _ string, names []string, from, to time.Time) ([]repository.AgentEvent, error) {
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	var out []repository.AgentEvent
	for _, e := range f.events {
		if want[e.Name] && !e.OccurredAt.Before(from) && !e.OccurredAt.After(to) {
			out = append(out, e)
		}
	}
	return out, f.err
}

const (
	agentRepo = "webwiebe/funnelbarn"
	sessionA  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sessionB  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// agentScript builds the agents project's events for ISO week 2026-W41
// (Monday 5 October to Sunday 11 October).
type agentScript struct{ events []repository.AgentEvent }

func oct(day, hour, minute int) time.Time {
	return time.Date(2026, time.October, day, hour, minute, 0, 0, time.UTC)
}

func issueRef(n string) string { return agentRepo + "#" + n }

func (s *agentScript) gh(name string, at time.Time, issue string, props map[string]any) {
	p := map[string]any{"source": "github", "repo": agentRepo}
	for k, v := range props {
		p[k] = v
	}
	hash := ""
	if issue != "" {
		p["issue_ref"] = issueRef(issue)
		hash = enrich.HashUserID(issueRef(issue))
	}
	s.events = append(s.events, repository.AgentEvent{Name: name, OccurredAt: at, SessionID: "0123456789abcdef0123456789abcdef", UserIDHash: hash, Properties: p})
}

func (s *agentScript) hook(name string, at time.Time, session, issue string) {
	p := map[string]any{"hook": "PostToolUse", "repo": agentRepo, "branch": "issue-" + issue + "/work", "source": nil}
	hash := ""
	if issue != "" {
		hash = enrich.HashUserID(issueRef(issue))
	}
	s.events = append(s.events, repository.AgentEvent{Name: name, OccurredAt: at, SessionID: session, UserIDHash: hash, Properties: p})
}

func (s *agentScript) pr(name string, at time.Time, issue string, n float64, branch, head string, extra map[string]any) {
	p := map[string]any{"pr": n, "branch": branch, "head_sha": head}
	for k, v := range extra {
		p[k] = v
	}
	s.gh(name, at, issue, p)
}

func (s *agentScript) ci(at time.Time, n float64, head, conclusion string, suite float64) {
	s.gh("ci_result", at, "", map[string]any{"pr": n, "head_sha": head, "conclusion": conclusion, "app": "github-actions", "check_suite_id": suite})
}

func (s *agentScript) workflow(at time.Time, name, conclusion, head string) {
	s.gh("workflow_completed", at, "", map[string]any{"workflow": name, "conclusion": conclusion, "head_sha": head, "run_attempt": float64(1)})
}

func weekScript() []repository.AgentEvent {
	var s agentScript
	// Issue 1 (session A): refined without a question, first-pass green,
	// shipped by a deploy that stays healthy.
	s.gh("issue_opened", oct(1, 10, 0), "1", nil)
	s.hook("commit_made", oct(5, 8, 55), sessionA, "1")
	s.gh("issue_refined", oct(5, 9, 0), "1", nil)
	s.pr("pr_opened", oct(5, 10, 0), "1", 10, "issue-1/work", "h10", nil)
	s.ci(oct(5, 10, 20), 10, "h10", "success", 1)
	s.pr("pr_merged", oct(5, 11, 0), "1", 10, "issue-1/work", "h10", map[string]any{"merge_sha": "m10"})
	s.hook("prod_deploy_started", oct(5, 11, 30), sessionA, "1")
	s.workflow(oct(5, 11, 45), "Deploy Production", "success", "m10")

	// Issue 2 (session B): a needs-you question, a red first CI run, a gate
	// that caught the merge, a failed deploy, then a deploy that was reverted.
	s.gh("issue_opened", oct(6, 9, 0), "2", nil)
	s.gh("needs_you_raised", oct(6, 10, 0), "2", nil)
	s.gh("needs_you_answered", oct(6, 14, 0), "2", nil)
	s.gh("issue_refined", oct(6, 15, 0), "2", nil)
	s.hook("commit_made", oct(6, 15, 30), sessionB, "2")
	s.pr("pr_opened", oct(6, 16, 0), "2", 11, "issue-2/work", "h11a", nil)
	s.ci(oct(6, 16, 20), 11, "h11a", "failure", 2)
	s.ci(oct(6, 17, 0), 11, "h11b", "success", 3)
	s.pr("pr_merged", oct(6, 17, 30), "2", 11, "issue-2/work", "h11b", map[string]any{"merge_sha": "m11"})
	s.workflow(oct(6, 17, 50), "Build and Test", "failure", "m11")
	s.hook("prod_deploy_started", oct(6, 18, 0), sessionB, "2")
	s.workflow(oct(6, 18, 10), "Deploy Production", "failure", "m11")
	s.pr("pr_opened", oct(6, 19, 0), "2", 12, "issue-2/work", "h12", nil)
	s.ci(oct(6, 19, 10), 12, "h12", "success", 4)
	s.pr("pr_merged", oct(6, 19, 30), "2", 12, "issue-2/work", "h12", map[string]any{"merge_sha": "m12"})
	s.hook("prod_deploy_started", oct(6, 19, 50), sessionB, "2")
	s.workflow(oct(6, 20, 0), "Deploy Production", "success", "m12")
	s.pr("pr_merged", oct(7, 8, 0), "", 13, "revert-12-issue-2/work", "h13", map[string]any{"merge_sha": "m13"})

	// Issue 3 (session A): merged with a red suite, an open question, and a
	// deploy still inside its health window at the end of the week.
	s.gh("issue_opened", oct(8, 9, 0), "3", nil)
	s.hook("commit_made", oct(8, 9, 50), sessionA, "3")
	s.pr("pr_opened", oct(8, 10, 0), "3", 14, "issue-3/work", "h14", nil)
	s.ci(oct(8, 10, 10), 14, "h14", "failure", 5)
	s.pr("pr_merged", oct(8, 10, 30), "3", 14, "issue-3/work", "h14", map[string]any{"merge_sha": "m14"})
	s.gh("needs_you_raised", oct(10, 12, 0), "3", nil)
	s.hook("prod_deploy_started", oct(11, 19, 55), sessionA, "3")
	s.workflow(oct(11, 20, 0), "Deploy Production", "success", "m14")

	// A GitHub deployment of this repo is ignored (its deploy workflow
	// already reports); another repo's production deployment counts, with
	// no session to credit.
	s.gh("deployment_status", oct(5, 11, 46), "", map[string]any{"environment": "production", "state": "success", "sha": "m10"})
	s.events = append(s.events, repository.AgentEvent{Name: "deployment_status", OccurredAt: oct(9, 12, 0),
		Properties: map[string]any{"source": "github", "repo": "other/repo", "environment": "Production", "state": "success", "sha": "o1"}})
	return s.events
}

func newAgentService(events []repository.AgentEvent) *service.AgentService {
	return service.NewAgentService(fakeAgentQueries{events: events}).
		WithClock(func() time.Time { return oct(12, 10, 0) })
}

func week41(t *testing.T) (time.Time, time.Time) {
	t.Helper()
	from, to, err := service.ParseISOWeek("2026-W41")
	require.NoError(t, err)
	return from, to
}

func TestAgentScoreboard_ScoresEveryRule(t *testing.T) {
	from, to := week41(t)
	sb, err := newAgentService(weekScript()).Scoreboard(context.Background(), "p", from, to, "")
	require.NoError(t, err)

	type row struct {
		session, rule, ref string
		points             int
	}
	var got []row
	for _, e := range sb.Entries {
		got = append(got, row{e.SessionID, e.Rule, e.Ref, e.Points})
	}
	assert.Equal(t, []row{
		{sessionA, service.RuleRefinedWithoutOwner, issueRef("1"), 2},
		{sessionA, service.RuleFirstPassGreen, agentRepo + "#10", 5},
		{sessionA, service.RuleDeployHealthy, agentRepo + "@m10", 10},
		{sessionB, service.RuleStagingCaught, agentRepo + "@m11", 2},
		{sessionB, service.RuleDeployBroken, agentRepo + "@m11", -20},
		{sessionB, service.RuleFirstPassGreen, agentRepo + "#12", 5},
		{sessionB, service.RuleDeployBroken, agentRepo + "@m12", -20},
		{sessionA, service.RuleRedBuildMerged, agentRepo + "#14", -10},
		{"", service.RuleDeployHealthy, "other/repo@o1", 10},
	}, got)

	require.Len(t, sb.Sessions, 2)
	a, b := sb.Sessions[0], sb.Sessions[1]
	assert.Equal(t, sessionA, a.SessionID)
	assert.Equal(t, 7, a.Points)
	assert.Equal(t, 0, a.FirstPassStreak, "A's last merge was red on its first run")
	assert.Equal(t, 1, a.BestFirstPassStreak)
	assert.Equal(t, sessionB, b.SessionID)
	assert.Equal(t, -33, b.Points)
	assert.Equal(t, 1, b.FirstPassStreak)
	assert.Equal(t, map[string]int{service.RuleStagingCaught: 2, service.RuleDeployBroken: -40, service.RuleFirstPassGreen: 5}, b.ByRule)
	assert.Equal(t, 10, sb.UnattributedPoints)

	require.Len(t, sb.Pending, 1, "the Sunday deploy is inside its 24h window")
	assert.Equal(t, sessionA, sb.Pending[0].SessionID)
	assert.Equal(t, agentRepo+"@m14", sb.Pending[0].Ref)
	require.Len(t, sb.Unscored, 1)
	assert.Equal(t, service.RuleBugBarnIssue, sb.Unscored[0].Rule)
}

func TestAgentScoreboard_FiltersOnSession(t *testing.T) {
	from, to := week41(t)
	sb, err := newAgentService(weekScript()).Scoreboard(context.Background(), "p", from, to, sessionB)
	require.NoError(t, err)
	require.Len(t, sb.Sessions, 1)
	assert.Equal(t, sessionB, sb.Sessions[0].SessionID)
	assert.Len(t, sb.Entries, 4)
	assert.Empty(t, sb.Pending)
	assert.Zero(t, sb.UnattributedPoints)
}

func TestAgentScoreboard_DeployScoresOnceTheWindowCloses(t *testing.T) {
	from, to := week41(t)
	svc := service.NewAgentService(fakeAgentQueries{events: weekScript()}).
		WithClock(func() time.Time { return oct(13, 0, 0) })
	sb, err := svc.Scoreboard(context.Background(), "p", from, to, sessionA)
	require.NoError(t, err)
	assert.Empty(t, sb.Pending)
	assert.Equal(t, 17, sb.Sessions[0].Points)
}

func TestAgentLeadTimes(t *testing.T) {
	from, to := week41(t)
	lt, err := newAgentService(weekScript()).LeadTimes(context.Background(), "p", from, to, "")
	require.NoError(t, err)

	issue1 := int64((4*24*time.Hour + 105*time.Minute) / time.Second)
	issue2 := int64(11 * time.Hour / time.Second)
	issue3 := int64((3*24*time.Hour + 11*time.Hour) / time.Second)
	assert.Equal(t, service.DurationStats{Count: 3, MedianSeconds: issue3, P90Seconds: issue1}, lt.IssueToProd)
	require.Len(t, lt.Issues, 3)
	assert.Equal(t, issueRef("1"), lt.Issues[0].IssueRef)
	assert.Equal(t, issue1, *lt.Issues[0].LeadSeconds)
	assert.Equal(t, oct(6, 20, 0), lt.Issues[1].ShippedAt, "the failed deploy does not ship issue 2")
	assert.Equal(t, issue2, *lt.Issues[1].LeadSeconds)
	assert.Equal(t, []service.AgentWeekStats{{Week: "2026-W41", DurationStats: lt.IssueToProd}}, lt.Weekly)
	assert.Equal(t, []service.AgentDayCount{{Date: "2026-10-05", Count: 1}, {Date: "2026-10-06", Count: 1}, {Date: "2026-10-11", Count: 1}}, lt.ShippedPerDay)

	assert.Equal(t, service.DurationStats{Count: 4, MedianSeconds: 1800, P90Seconds: 5400}, lt.PRToMerge)
	assert.Equal(t, []service.AgentRepoCI{{Repo: agentRepo, PRs: 4, FirstPassGreen: 2, Rate: 0.5}}, lt.FirstPassCI)

	assert.Equal(t, service.DurationStats{Count: 1, MedianSeconds: 14400, P90Seconds: 14400}, lt.OwnerWait)
	require.Len(t, lt.OpenWaits, 1)
	assert.Equal(t, issueRef("3"), lt.OpenWaits[0].IssueRef)
	assert.Equal(t, int64(36*3600-1), lt.OpenWaits[0].WaitingSeconds, "measured to the end of the range")
}

func TestAgentLeadTimes_FiltersOnRepo(t *testing.T) {
	from, to := week41(t)
	lt, err := newAgentService(weekScript()).LeadTimes(context.Background(), "p", from, to, "other/repo")
	require.NoError(t, err)
	assert.Zero(t, lt.IssueToProd.Count)
	assert.Empty(t, lt.Issues)
	assert.Empty(t, lt.FirstPassCI)
	assert.Empty(t, lt.OpenWaits)
}

func TestAgentService_RejectsBadRanges(t *testing.T) {
	svc := newAgentService(nil)
	_, err := svc.LeadTimes(context.Background(), "p", oct(10, 0, 0), oct(9, 0, 0), "")
	assert.ErrorIs(t, err, domain.ErrValidation)
	_, err = svc.Scoreboard(context.Background(), "p", oct(1, 0, 0), oct(1, 0, 0).AddDate(2, 0, 0), "")
	assert.ErrorIs(t, err, domain.ErrValidation)
}

func TestAgentService_PassesStoreErrorsThrough(t *testing.T) {
	boom := errors.New("boom")
	svc := service.NewAgentService(fakeAgentQueries{err: boom})
	_, err := svc.Scoreboard(context.Background(), "p", oct(1, 0, 0), oct(2, 0, 0), "")
	assert.ErrorIs(t, err, boom)
}

func TestParseISOWeek(t *testing.T) {
	from, to, err := service.ParseISOWeek("2026-w41")
	require.NoError(t, err)
	assert.Equal(t, oct(5, 0, 0), from)
	assert.Equal(t, oct(12, 0, 0).Add(-time.Nanosecond), to)

	from, _, err = service.ParseISOWeek("2026-W01")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2025, time.December, 29, 0, 0, 0, 0, time.UTC), from)

	for _, bad := range []string{"", "2026", "2026-41", "2026-W00", "2025-W53", "W41-2026"} {
		_, _, err := service.ParseISOWeek(bad)
		assert.ErrorIs(t, err, domain.ErrValidation, bad)
	}
	assert.Equal(t, "2026-W41", service.CurrentISOWeek(oct(11, 23, 0)))
}
