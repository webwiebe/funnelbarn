package service

import (
	"context"
	"sort"
	"time"
)

// Scoreboard rules (#331 section 3, revised in #333).
const (
	RuleDeployHealthy       = "deploy_healthy"
	RuleDeployBroken        = "deploy_broken"
	RuleBugBarnIssue        = "bugbarn_issue"
	RuleFirstPassGreen      = "first_pass_green"
	RuleRefinedWithoutOwner = "refined_without_owner"
	RuleStagingCaught       = "staging_caught"
	RuleRedBuildMerged      = "red_build_merged"
)

// AgentRulePoints are the points each rule awards.
var AgentRulePoints = map[string]int{
	RuleDeployHealthy:       10,
	RuleDeployBroken:        -20,
	RuleBugBarnIssue:        -10,
	RuleFirstPassGreen:      5,
	RuleRefinedWithoutOwner: 2,
	RuleStagingCaught:       2,
	RuleRedBuildMerged:      -10,
}

// AgentScoreEntry is one scored event. SessionID is "" when no agent
// session could be attributed.
type AgentScoreEntry struct {
	At        time.Time `json:"at"`
	SessionID string    `json:"session_id"`
	Rule      string    `json:"rule"`
	Points    int       `json:"points"`
	Ref       string    `json:"ref"`
	Detail    string    `json:"detail"`
}

// AgentSessionScore totals one agent session.
type AgentSessionScore struct {
	SessionID string         `json:"session_id"`
	Points    int            `json:"points"`
	Entries   int            `json:"entries"`
	ByRule    map[string]int `json:"by_rule"`
	// FirstPassStreak is the run of consecutive first-pass green merges the
	// session ended the range on; BestFirstPassStreak the longest in it.
	FirstPassStreak     int `json:"first_pass_streak"`
	BestFirstPassStreak int `json:"best_first_pass_streak"`
}

// AgentUnscoredRule names a rule the scoreboard cannot score yet, and why.
type AgentUnscoredRule struct {
	Rule   string `json:"rule"`
	Reason string `json:"reason"`
}

// AgentScoreboard is the scoreboard for a range.
type AgentScoreboard struct {
	From     time.Time           `json:"from"`
	To       time.Time           `json:"to"`
	Sessions []AgentSessionScore `json:"sessions"`
	// UnattributedPoints sums the entries no session could be matched to.
	UnattributedPoints int               `json:"unattributed_points"`
	Entries            []AgentScoreEntry `json:"entries"`
	// Pending lists deploys still inside their 24 h health window: they
	// score once the window closes.
	Pending  []AgentScoreEntry   `json:"pending"`
	Unscored []AgentUnscoredRule `json:"unscored"`
}

// Scoreboard scores the agent sessions over [from, to]. session, when set,
// limits the result to that session's entries.
func (s *AgentService) Scoreboard(ctx context.Context, projectID string, from, to time.Time, session string) (AgentScoreboard, error) {
	m, err := s.load(ctx, projectID, from, to)
	if err != nil {
		return AgentScoreboard{}, err
	}
	sc := &scorer{m: m, from: from, to: to, now: s.now()}
	sc.scoreDeploys()
	sc.scoreMerges()
	sc.scoreRefinements()
	sc.scoreGateFailures()
	return sc.result(session), nil
}

type scorer struct {
	m        *agentModel
	from, to time.Time
	now      time.Time
	entries  []AgentScoreEntry
	pending  []AgentScoreEntry
	// streak tracks first-pass results per session in merge order.
	streaks map[string][]bool
}

func (sc *scorer) in(t time.Time) bool { return !t.Before(sc.from) && !t.After(sc.to) }

func (sc *scorer) add(at time.Time, session, rule, ref, detail string) {
	sc.entries = append(sc.entries, AgentScoreEntry{At: at, SessionID: session, Rule: rule,
		Points: AgentRulePoints[rule], Ref: ref, Detail: detail})
}

// scoreDeploys scores each production deploy finished in the range: -20 when
// it failed (smoke or rollback make the deploy workflow fail) or a revert or
// hotfix merged in the repo within 24 h, else +10 once 24 h have passed.
func (sc *scorer) scoreDeploys() {
	for _, d := range sc.m.deploys {
		if !sc.in(d.at) {
			continue
		}
		session := sc.m.deploySession(d)
		ref := d.repo + "@" + shortSHA(d.sha)
		switch {
		case !d.ok:
			sc.add(d.at, session, RuleDeployBroken, ref, d.workflow+" failed")
		case sc.revertAfter(d) != nil:
			sc.add(d.at, session, RuleDeployBroken, ref, "revert or hotfix "+sc.revertAfter(d).key+" merged within 24h")
		case sc.now.Before(d.at.Add(agentHealthWindow)):
			sc.pending = append(sc.pending, AgentScoreEntry{At: d.at, SessionID: session, Rule: RuleDeployHealthy,
				Points: AgentRulePoints[RuleDeployHealthy], Ref: ref, Detail: "healthy so far; scores 24h after the deploy"})
		default:
			sc.add(d.at, session, RuleDeployHealthy, ref, d.workflow+" healthy for 24h")
		}
	}
}

func (sc *scorer) revertAfter(d agentDeploy) *agentPR {
	for _, pr := range sortedPRs(sc.m.prs) {
		if pr.repo != d.repo || pr.mergedAt.IsZero() || !isRevertOrHotfix(pr.branch) {
			continue
		}
		if pr.mergedAt.After(d.at) && !pr.mergedAt.After(d.at.Add(agentHealthWindow)) {
			return pr
		}
	}
	return nil
}

// scoreMerges scores pull requests merged in the range: +5 when the first CI
// result was green, -10 when a suite on the merged head was red at merge.
func (sc *scorer) scoreMerges() {
	sc.streaks = map[string][]bool{}
	for _, pr := range sortedPRs(sc.m.prs) {
		if !sc.in(pr.mergedAt) {
			continue
		}
		session := sc.m.attribute(pr.hash, pr.repo, pr.branch, pr.mergedAt)
		if first, ok := sc.m.firstCI(pr); ok {
			green := first.conclusion == "success"
			if green {
				sc.add(pr.mergedAt, session, RuleFirstPassGreen, pr.key, "first CI run green")
			}
			sc.streaks[session] = append(sc.streaks[session], green)
		}
		if green, known := sc.m.greenAtMerge(pr); known && !green {
			sc.add(pr.mergedAt, session, RuleRedBuildMerged, pr.key, "merged with a red check suite")
		}
	}
}

// scoreRefinements gives +2 for an issue refined in the range without a
// needs-you question before it.
func (sc *scorer) scoreRefinements() {
	for _, iss := range sortedIssues(sc.m.issues) {
		if !sc.in(iss.refinedAt) {
			continue
		}
		asked := false
		for _, r := range iss.raised {
			if !r.After(iss.refinedAt) {
				asked = true
			}
		}
		if !asked {
			session := sc.m.attribute(iss.hash, iss.repo, "", iss.refinedAt)
			sc.add(iss.refinedAt, session, RuleRefinedWithoutOwner, iss.ref, "refined without an owner decision")
		}
	}
}

// scoreGateFailures gives +2 for a staging, E2E or main build failure: the
// gate stopped a release before production. It goes to the session whose
// merge produced the failing commit.
func (sc *scorer) scoreGateFailures() {
	for _, g := range sc.m.gateFailures {
		if !sc.in(g.at) {
			continue
		}
		session := ""
		if pr := sc.m.prByMergeSHA(g.repo, g.sha); pr != nil {
			session = sc.m.attribute(pr.hash, pr.repo, pr.branch, pr.mergedAt)
		}
		sc.add(g.at, session, RuleStagingCaught, g.repo+"@"+shortSHA(g.sha), g.workflow+" failed before production")
	}
}

func (sc *scorer) result(session string) AgentScoreboard {
	out := AgentScoreboard{From: sc.from, To: sc.to, Entries: []AgentScoreEntry{}, Pending: []AgentScoreEntry{},
		Unscored: []AgentUnscoredRule{{Rule: RuleBugBarnIssue,
			Reason: "BugBarn issues are not in the agents project yet; a new error-level issue after a release is not scored"}}}
	byID := map[string]*AgentSessionScore{}
	for _, e := range sortedEntries(sc.entries) {
		if session != "" && e.SessionID != session {
			continue
		}
		out.Entries = append(out.Entries, e)
		if e.SessionID == "" {
			out.UnattributedPoints += e.Points
			continue
		}
		ss := byID[e.SessionID]
		if ss == nil {
			ss = &AgentSessionScore{SessionID: e.SessionID, ByRule: map[string]int{}}
			byID[e.SessionID] = ss
		}
		ss.Points += e.Points
		ss.Entries++
		ss.ByRule[e.Rule] += e.Points
	}
	for _, e := range sortedEntries(sc.pending) {
		if session == "" || e.SessionID == session {
			out.Pending = append(out.Pending, e)
		}
	}
	out.Sessions = make([]AgentSessionScore, 0, len(byID))
	for id, ss := range byID {
		ss.FirstPassStreak, ss.BestFirstPassStreak = streaks(sc.streaks[id])
		out.Sessions = append(out.Sessions, *ss)
	}
	sort.Slice(out.Sessions, func(i, j int) bool {
		a, b := out.Sessions[i], out.Sessions[j]
		if a.Points != b.Points {
			return a.Points > b.Points
		}
		return a.SessionID < b.SessionID
	})
	return out
}

// streaks returns the trailing and the longest run of true values.
func streaks(results []bool) (current, best int) {
	for _, green := range results {
		if green {
			current++
			best = max(best, current)
		} else {
			current = 0
		}
	}
	return current, best
}

func sortedEntries(es []AgentScoreEntry) []AgentScoreEntry {
	out := append([]AgentScoreEntry(nil), es...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.Before(out[j].At)
		}
		if out[i].Ref != out[j].Ref {
			return out[i].Ref < out[j].Ref
		}
		return out[i].Rule < out[j].Rule
	})
	return out
}

func sortedPRs(prs map[string]*agentPR) []*agentPR {
	out := make([]*agentPR, 0, len(prs))
	for _, pr := range prs {
		out = append(out, pr)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].mergedAt.Equal(out[j].mergedAt) {
			return out[i].mergedAt.Before(out[j].mergedAt)
		}
		return out[i].key < out[j].key
	})
	return out
}

func sortedIssues(issues map[string]*agentIssue) []*agentIssue {
	out := make([]*agentIssue, 0, len(issues))
	for _, iss := range issues {
		out = append(out, iss)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ref < out[j].ref })
	return out
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
