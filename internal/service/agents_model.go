package service

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// The agents project (#331) holds two event sources under the same names:
// GitHub webhook events (property source=github) and the workstation hook's
// events (session_id is the agent session). agentModel joins them.

// agentEventNames are the event names the scoring reads. The hook's other,
// high-volume events (subagent_finished and the like) are never loaded.
var agentEventNames = []string{
	"issue_opened", "issue_refined", "needs_you_raised", "needs_you_answered",
	"pr_opened", "pr_merged", "ci_result", "deployment_status", "workflow_completed",
	"commit_made", "prod_deploy_started",
}

var (
	// prodDeployWorkflow matches a workflow that deploys to production, such
	// as "Deploy Production".
	prodDeployWorkflow = regexp.MustCompile(`(?i)deploy.*prod|prod.*deploy`)
	// gateWorkflow matches a workflow that gates a release before production:
	// the staging deploy, E2E or the main build.
	gateWorkflow = regexp.MustCompile(`(?i)staging|e2e|build.and.test`)
	// prodEnvironment matches a GitHub deployment environment for production.
	prodEnvironment = regexp.MustCompile(`(?i)^prod`)
)

type agentIssue struct {
	ref, repo, hash string
	openedAt        time.Time
	refinedAt       time.Time
	raised          []time.Time
	answered        []time.Time
	prs             []*agentPR
	shippedAt       time.Time
}

type agentPR struct {
	key, repo, issueRef, hash string
	number                    int
	branch, headSHA, mergeSHA string
	openedAt, mergedAt        time.Time
}

type agentCIRun struct {
	at         time.Time
	pr         int
	suite      string
	conclusion string
}

type agentDeploy struct {
	repo, workflow, sha string
	at                  time.Time
	ok                  bool
	fromWorkflow        bool
}

type agentHook struct {
	at                     time.Time
	name, session, hash    string
	repo, branch, issueRef string
}

type agentModel struct {
	issues       map[string]*agentIssue
	prs          map[string]*agentPR
	ciBySHA      map[string][]agentCIRun // repo@sha
	ciByPR       map[string][]agentCIRun // repo#pr
	deploys      []agentDeploy
	gateFailures []agentDeploy
	hooks        []agentHook
	deployStarts []agentHook
}

func buildAgentModel(events []repository.AgentEvent) *agentModel {
	m := &agentModel{
		issues:  map[string]*agentIssue{},
		prs:     map[string]*agentPR{},
		ciBySHA: map[string][]agentCIRun{},
		ciByPR:  map[string][]agentCIRun{},
	}
	for _, e := range events {
		if propStr(e.Properties, "source") == "github" {
			m.addGitHub(e)
		} else {
			m.addHook(e)
		}
	}
	m.finish()
	return m
}

func (m *agentModel) addGitHub(e repository.AgentEvent) {
	p := e.Properties
	switch e.Name {
	case "issue_opened", "issue_refined", "needs_you_raised", "needs_you_answered":
		m.addIssueEvent(e)
	case "pr_opened", "pr_merged":
		m.addPR(e)
	case "ci_result":
		run := agentCIRun{at: e.OccurredAt, pr: propInt(p, "pr"), conclusion: propStr(p, "conclusion"),
			suite: propStr(p, "check_suite_id") + "/" + propStr(p, "app")}
		repo := propStr(p, "repo")
		m.ciBySHA[repo+"@"+propStr(p, "head_sha")] = append(m.ciBySHA[repo+"@"+propStr(p, "head_sha")], run)
		m.ciByPR[prKey(repo, run.pr)] = append(m.ciByPR[prKey(repo, run.pr)], run)
	case "deployment_status":
		m.addDeploymentStatus(e)
	case "workflow_completed":
		m.addWorkflow(e)
	}
}

func (m *agentModel) issue(ref, repo, hash string) *agentIssue {
	iss := m.issues[ref]
	if iss == nil {
		iss = &agentIssue{ref: ref, repo: repo}
		m.issues[ref] = iss
	}
	if iss.hash == "" {
		iss.hash = hash
	}
	return iss
}

func (m *agentModel) addIssueEvent(e repository.AgentEvent) {
	ref := propStr(e.Properties, "issue_ref")
	if ref == "" {
		return
	}
	iss := m.issue(ref, propStr(e.Properties, "repo"), e.UserIDHash)
	switch e.Name {
	case "issue_opened":
		if iss.openedAt.IsZero() {
			iss.openedAt = e.OccurredAt
		}
	case "issue_refined":
		if iss.refinedAt.IsZero() {
			iss.refinedAt = e.OccurredAt
		}
	case "needs_you_raised":
		iss.raised = append(iss.raised, e.OccurredAt)
	case "needs_you_answered":
		iss.answered = append(iss.answered, e.OccurredAt)
	}
}

func (m *agentModel) addPR(e repository.AgentEvent) {
	p := e.Properties
	repo, n := propStr(p, "repo"), propInt(p, "pr")
	key := prKey(repo, n)
	pr := m.prs[key]
	if pr == nil {
		pr = &agentPR{key: key, repo: repo, number: n}
		m.prs[key] = pr
	}
	pr.branch = propStr(p, "branch")
	pr.headSHA = propStr(p, "head_sha")
	if e.Name == "pr_opened" {
		pr.openedAt = e.OccurredAt
	} else {
		pr.mergedAt = e.OccurredAt
		pr.mergeSHA = propStr(p, "merge_sha")
	}
	if ref := propStr(p, "issue_ref"); ref != "" && pr.issueRef == "" {
		pr.issueRef, pr.hash = ref, e.UserIDHash
		iss := m.issue(ref, repo, e.UserIDHash)
		iss.prs = append(iss.prs, pr)
	}
}

func (m *agentModel) addDeploymentStatus(e repository.AgentEvent) {
	p := e.Properties
	if !prodEnvironment.MatchString(propStr(p, "environment")) {
		return
	}
	d := agentDeploy{repo: propStr(p, "repo"), workflow: "deployment " + propStr(p, "environment"),
		sha: propStr(p, "sha"), at: e.OccurredAt}
	switch propStr(p, "state") {
	case "success":
		d.ok = true
	case "failure", "error":
	default:
		return
	}
	m.deploys = append(m.deploys, d)
}

func (m *agentModel) addWorkflow(e repository.AgentEvent) {
	p := e.Properties
	name, conclusion := propStr(p, "workflow"), propStr(p, "conclusion")
	if conclusion != "success" && conclusion != "failure" {
		return // cancelled, skipped and the like say nothing about the release
	}
	d := agentDeploy{repo: propStr(p, "repo"), workflow: name, sha: propStr(p, "head_sha"),
		at: e.OccurredAt, ok: conclusion == "success", fromWorkflow: true}
	switch {
	case prodDeployWorkflow.MatchString(name):
		m.deploys = append(m.deploys, d)
	case gateWorkflow.MatchString(name) && !d.ok:
		m.gateFailures = append(m.gateFailures, d)
	}
}

func (m *agentModel) addHook(e repository.AgentEvent) {
	p := e.Properties
	h := agentHook{at: e.OccurredAt, name: e.Name, session: e.SessionID, hash: e.UserIDHash,
		repo: propStr(p, "repo"), branch: propStr(p, "branch"), issueRef: propStr(p, "issue")}
	m.hooks = append(m.hooks, h)
	if e.Name == "prod_deploy_started" {
		m.deployStarts = append(m.deployStarts, h)
	}
}

// finish drops deployment statuses for repos whose deploy workflow already
// reports (one deploy must not count twice) and marks each issue shipped at
// the first good production deploy of its repo after its first merge.
func (m *agentModel) finish() {
	byWorkflow := map[string]bool{}
	for _, d := range m.deploys {
		if d.fromWorkflow {
			byWorkflow[d.repo] = true
		}
	}
	kept := m.deploys[:0]
	for _, d := range m.deploys {
		if d.fromWorkflow || !byWorkflow[d.repo] {
			kept = append(kept, d)
		}
	}
	m.deploys = kept
	sort.SliceStable(m.deploys, func(i, j int) bool { return m.deploys[i].at.Before(m.deploys[j].at) })
	for _, iss := range m.issues {
		merged := iss.firstMerge()
		if merged == nil {
			continue
		}
		if d := m.firstGoodDeploy(merged.repo, merged.mergedAt); d != nil {
			iss.shippedAt = d.at
		}
	}
}

func (iss *agentIssue) firstMerge() *agentPR {
	var first *agentPR
	for _, pr := range iss.prs {
		if pr.mergedAt.IsZero() {
			continue
		}
		if first == nil || pr.mergedAt.Before(first.mergedAt) {
			first = pr
		}
	}
	return first
}

func (m *agentModel) firstGoodDeploy(repo string, after time.Time) *agentDeploy {
	for i := range m.deploys {
		d := &m.deploys[i]
		if d.repo == repo && d.ok && !d.at.Before(after) {
			return d
		}
	}
	return nil
}

// firstCI is the first CI result reported for a pull request.
func (m *agentModel) firstCI(pr *agentPR) (agentCIRun, bool) {
	runs := m.ciByPR[pr.key]
	if len(runs) == 0 {
		return agentCIRun{}, false
	}
	first := runs[0]
	for _, r := range runs[1:] {
		if r.at.Before(first.at) {
			first = r
		}
	}
	return first, true
}

// greenAtMerge reports whether every check suite on the merged head was
// green at merge time, using each suite's latest result. known is false when
// no suite reported for that head before the merge.
func (m *agentModel) greenAtMerge(pr *agentPR) (green, known bool) {
	latest := map[string]agentCIRun{}
	limit := pr.mergedAt.Add(agentAttributionSlack)
	for _, r := range m.ciBySHA[pr.repo+"@"+pr.headSHA] {
		if r.at.After(limit) {
			continue
		}
		if prev, ok := latest[r.suite]; !ok || !r.at.Before(prev.at) {
			latest[r.suite] = r
		}
	}
	if len(latest) == 0 {
		return false, false
	}
	for _, r := range latest {
		switch r.conclusion {
		case "success", "neutral", "skipped":
		default:
			return false, true
		}
	}
	return true, true
}

// attribute returns the agent session that last worked on an issue (same
// user_id_hash) or, failing that, on the same repo and feature branch, at or
// shortly after at. "" means no hook event matches.
func (m *agentModel) attribute(hash, repo, branch string, at time.Time) string {
	limit := at.Add(agentAttributionSlack)
	session := ""
	for _, h := range m.hooks {
		if h.at.After(limit) {
			break
		}
		byIssue := hash != "" && h.hash == hash
		byBranch := branch != "" && branch != "main" && h.repo == repo && h.branch == branch
		if byIssue || byBranch {
			session = h.session
		}
	}
	return session
}

// deploySession returns the session whose prod_deploy_started for repo is
// the latest one within agentDeployAttribution before the deploy finished.
func (m *agentModel) deploySession(d agentDeploy) string {
	session := ""
	for _, h := range m.deployStarts {
		if h.repo == d.repo && !h.at.After(d.at) && d.at.Sub(h.at) <= agentDeployAttribution {
			session = h.session
		}
	}
	return session
}

// prByMergeSHA finds the pull request whose merge produced sha in repo.
func (m *agentModel) prByMergeSHA(repo, sha string) *agentPR {
	for _, pr := range m.prs {
		if pr.repo == repo && sha != "" && pr.mergeSHA == sha {
			return pr
		}
	}
	return nil
}

func prKey(repo string, n int) string { return repo + "#" + strconv.Itoa(n) }

func propStr(p map[string]any, k string) string {
	switch v := p[k].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

func propInt(p map[string]any, k string) int {
	switch v := p[k].(type) {
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	}
	return 0
}

// isRevertOrHotfix reports whether a branch name marks a revert (GitHub's
// revert button names it revert-<pr>-<branch>) or a hotfix.
func isRevertOrHotfix(branch string) bool {
	b := strings.ToLower(branch)
	return strings.HasPrefix(b, "revert-") || strings.HasPrefix(b, "hotfix/") || strings.HasPrefix(b, "hotfix-")
}
