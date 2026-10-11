// Package ghwebhook turns GitHub webhook deliveries into FunnelBarn events for
// the agents delivery funnels (#331). It only maps payloads; signature checks
// and enqueueing live in the API handler.
package ghwebhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Event is one FunnelBarn event mapped from a delivery. It marshals to the
// body POST /api/v1/events accepts.
type Event struct {
	Name        string         `json:"name"`
	UserID      string         `json:"user_id,omitempty"`
	SessionID   string         `json:"session_id"`
	Environment string         `json:"environment"`
	Timestamp   time.Time      `json:"timestamp"`
	Properties  map[string]any `json:"properties"`
}

// Source is the value of the source property on every mapped event. The
// event environment stays production: environments are a fixed set, and an
// unknown one is filed as production anyway.
const Source = "github"

// Labels whose changes the funnels track.
const (
	LabelNeedsRefining = "needs-refining"
	LabelNeedsYou      = "needs-you"
)

// ValidSignature reports whether header (X-Hub-Signature-256) is the
// HMAC-SHA256 of body under secret. The compare is constant-time.
func ValidSignature(secret, header string, body []byte) bool {
	const prefix = "sha256="
	if secret == "" || !strings.HasPrefix(header, prefix) {
		return false
	}
	got, err := hex.DecodeString(header[len(prefix):])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// Sign returns the X-Hub-Signature-256 value for body, for tests and fixtures.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

type repository struct {
	FullName string `json:"full_name"`
}

type label struct {
	Name string `json:"name"`
}

type issue struct {
	Number      int       `json:"number"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Labels      []label   `json:"labels"`
	PullRequest *struct{} `json:"pull_request"`
}

type pullRequest struct {
	Number    int        `json:"number"`
	Body      string     `json:"body"`
	CreatedAt time.Time  `json:"created_at"`
	MergedAt  *time.Time `json:"merged_at"`
	Merged    bool       `json:"merged"`
	Head      struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	MergeCommitSHA string `json:"merge_commit_sha"`
}

type suitePR struct {
	Number int `json:"number"`
	Head   struct {
		Ref string `json:"ref"`
	} `json:"head"`
}

type delivery struct {
	Action      string       `json:"action"`
	Repository  repository   `json:"repository"`
	Issue       *issue       `json:"issue"`
	Label       *label       `json:"label"`
	PullRequest *pullRequest `json:"pull_request"`
	CheckSuite  *struct {
		ID           int64     `json:"id"`
		HeadBranch   string    `json:"head_branch"`
		HeadSHA      string    `json:"head_sha"`
		Conclusion   string    `json:"conclusion"`
		UpdatedAt    time.Time `json:"updated_at"`
		PullRequests []suitePR `json:"pull_requests"`
		App          struct {
			Slug string `json:"slug"`
		} `json:"app"`
	} `json:"check_suite"`
	Release *struct {
		TagName     string     `json:"tag_name"`
		PublishedAt *time.Time `json:"published_at"`
	} `json:"release"`
	Deployment *struct {
		Environment string `json:"environment"`
		Ref         string `json:"ref"`
		SHA         string `json:"sha"`
	} `json:"deployment"`
	DeploymentStatus *struct {
		State       string    `json:"state"`
		Environment string    `json:"environment"`
		CreatedAt   time.Time `json:"created_at"`
	} `json:"deployment_status"`
	WorkflowRun *struct {
		Name         string    `json:"name"`
		Event        string    `json:"event"`
		HeadBranch   string    `json:"head_branch"`
		HeadSHA      string    `json:"head_sha"`
		Conclusion   string    `json:"conclusion"`
		RunAttempt   int       `json:"run_attempt"`
		UpdatedAt    time.Time `json:"updated_at"`
		PullRequests []suitePR `json:"pull_requests"`
	} `json:"workflow_run"`
}

// Map turns one delivery into at most one event. kind is the X-GitHub-Event
// header. ok is false for deliveries the funnels do not track (other events
// and actions, ping), which the caller acknowledges without storing.
func Map(kind string, body []byte) (ev Event, ok bool, err error) {
	var d delivery
	if err := json.Unmarshal(body, &d); err != nil {
		return Event{}, false, fmt.Errorf("decode %s payload: %w", kind, err)
	}
	repo := d.Repository.FullName
	switch kind {
	case "issues":
		return mapIssue(d, repo)
	case "pull_request":
		return mapPullRequest(d, repo)
	case "check_suite":
		return mapCheckSuite(d, repo)
	case "release":
		return mapRelease(d, repo)
	case "deployment_status":
		return mapDeploymentStatus(d, repo)
	case "workflow_run":
		return mapWorkflowRun(d, repo)
	}
	return Event{}, false, nil
}

func mapRelease(d delivery, repo string) (Event, bool, error) {
	if d.Action != "published" || d.Release == nil {
		return Event{}, false, nil
	}
	ev := newEvent("release_published", "", repo, timeOr(d.Release.PublishedAt))
	ev.Properties["tag"] = d.Release.TagName
	return ev, true, nil
}

func mapDeploymentStatus(d delivery, repo string) (Event, bool, error) {
	if d.DeploymentStatus == nil || d.Deployment == nil {
		return Event{}, false, nil
	}
	ev := newEvent("deployment_status", "", repo, d.DeploymentStatus.CreatedAt)
	env := d.DeploymentStatus.Environment
	if env == "" {
		env = d.Deployment.Environment
	}
	ev.Properties["environment"] = env
	ev.Properties["state"] = d.DeploymentStatus.State
	ev.Properties["ref"] = d.Deployment.Ref
	ev.Properties["sha"] = d.Deployment.SHA
	return ev, true, nil
}

// issueEventName names the event for an issues delivery, or "" when the
// funnels do not track that action and label.
func issueEventName(action, labelName string) string {
	switch {
	case action == "opened":
		return "issue_opened"
	case action == "unlabeled" && labelName == LabelNeedsRefining:
		return "issue_refined"
	case action == "labeled" && labelName == LabelNeedsYou:
		return "needs_you_raised"
	case action == "unlabeled" && labelName == LabelNeedsYou:
		return "needs_you_answered"
	}
	return ""
}

func mapIssue(d delivery, repo string) (Event, bool, error) {
	if d.Issue == nil || d.Issue.PullRequest != nil {
		return Event{}, false, nil
	}
	ref := issueRef(repo, d.Issue.Number)
	labelName := ""
	if d.Label != nil {
		labelName = d.Label.Name
	}
	name := issueEventName(d.Action, labelName)
	if name == "" {
		return Event{}, false, nil
	}
	at := d.Issue.UpdatedAt
	if name == "issue_opened" {
		at = d.Issue.CreatedAt
	}
	ev := newEvent(name, ref, repo, at)
	if name == "issue_opened" {
		labels := make([]string, 0, len(d.Issue.Labels))
		for _, l := range d.Issue.Labels {
			labels = append(labels, l.Name)
		}
		ev.Properties["labels"] = strings.Join(labels, ",")
	}
	return ev, true, nil
}

func mapPullRequest(d delivery, repo string) (Event, bool, error) {
	pr := d.PullRequest
	if pr == nil {
		return Event{}, false, nil
	}
	ref := LinkedIssue(repo, pr.Body, pr.Head.Ref)
	var ev Event
	switch {
	case d.Action == "opened":
		ev = newEvent("pr_opened", ref, repo, pr.CreatedAt)
	case d.Action == "closed" && pr.Merged:
		ev = newEvent("pr_merged", ref, repo, timeOr(pr.MergedAt))
		ev.Properties["merge_sha"] = pr.MergeCommitSHA
	default:
		return Event{}, false, nil
	}
	ev.Properties["pr"] = pr.Number
	ev.Properties["branch"] = pr.Head.Ref
	ev.Properties["head_sha"] = pr.Head.SHA
	return ev, true, nil
}

// mapCheckSuite maps a completed suite on a pull request to ci_result. The
// payload has no attempt counter, so first_attempt is left to the read side:
// the first ci_result of a head_sha is its first attempt.
func mapCheckSuite(d delivery, repo string) (Event, bool, error) {
	cs := d.CheckSuite
	if d.Action != "completed" || cs == nil || len(cs.PullRequests) == 0 {
		return Event{}, false, nil
	}
	pr := cs.PullRequests[0]
	ev := newEvent("ci_result", LinkedIssue(repo, "", cs.HeadBranch), repo, cs.UpdatedAt)
	ev.Properties["pr"] = pr.Number
	ev.Properties["branch"] = cs.HeadBranch
	ev.Properties["head_sha"] = cs.HeadSHA
	ev.Properties["conclusion"] = cs.Conclusion
	ev.Properties["app"] = cs.App.Slug
	ev.Properties["check_suite_id"] = cs.ID
	return ev, true, nil
}

// mapWorkflowRun maps a completed run, which the scoring needs for the deploy
// and smoke workflows. Every workflow is kept; the read side picks by name.
func mapWorkflowRun(d delivery, repo string) (Event, bool, error) {
	run := d.WorkflowRun
	if d.Action != "completed" || run == nil {
		return Event{}, false, nil
	}
	ev := newEvent("workflow_completed", LinkedIssue(repo, "", run.HeadBranch), repo, run.UpdatedAt)
	ev.Properties["workflow"] = run.Name
	ev.Properties["trigger"] = run.Event
	ev.Properties["branch"] = run.HeadBranch
	ev.Properties["head_sha"] = run.HeadSHA
	ev.Properties["conclusion"] = run.Conclusion
	ev.Properties["run_attempt"] = run.RunAttempt
	ev.Properties["first_attempt"] = run.RunAttempt <= 1
	if len(run.PullRequests) > 0 {
		ev.Properties["pr"] = run.PullRequests[0].Number
	}
	return ev, true, nil
}

// newEvent builds an event keyed on the issue ref when there is one. Every
// event of one issue shares a session derived from the ref, so a session
// funnel over the agents project also reads per issue.
func newEvent(name, ref, repo string, at time.Time) Event {
	key := ref
	if key == "" {
		key = repo
	}
	sum := sha256.Sum256([]byte("github:" + key))
	props := map[string]any{"repo": repo, "source": Source}
	if ref != "" {
		props["issue_ref"] = ref
	}
	return Event{
		Name:        name,
		UserID:      ref,
		SessionID:   hex.EncodeToString(sum[:16]),
		Environment: "production",
		Timestamp:   at.UTC(),
		Properties:  props,
	}
}

func timeOr(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func issueRef(repo string, n int) string {
	return repo + "#" + strconv.Itoa(n)
}

var (
	closingRef  = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s*:?\s+([\w.-]+/[\w.-]+)?#(\d+)\b`)
	issueBranch = regexp.MustCompile(`^issue-(\d+)/`)
)

// LinkedIssue returns the issue ref ("owner/repo#n") a pull request belongs
// to: the first closing keyword in its body (Closes, Fixes or Resolves, any
// case, optionally naming another repo), else an issue-<n>/ branch. It is ""
// when the PR names neither.
func LinkedIssue(repo, body, branch string) string {
	if m := closingRef.FindStringSubmatch(body); m != nil {
		target := repo
		if m[1] != "" {
			target = m[1]
		}
		n, _ := strconv.Atoi(m[2])
		return issueRef(target, n)
	}
	if m := issueBranch.FindStringSubmatch(branch); m != nil {
		n, _ := strconv.Atoi(m[1])
		return issueRef(repo, n)
	}
	return ""
}
