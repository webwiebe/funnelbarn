package ghwebhook_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/ghwebhook"
	"github.com/wiebe-xyz/funnelbarn/internal/session"
)

// The fixtures are real objects from webwiebe/funnelbarn (issue #331, PR #339,
// its check suite, release v0.6.84, a deployment and the production deploy
// run of v0.6.84) wrapped in the webhook envelope and trimmed.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	require.NoError(t, err)
	return b
}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

const repo = "webwiebe/funnelbarn"

func TestMap_EveryTrackedDelivery(t *testing.T) {
	cases := []struct {
		fixture, kind, name, userID string
		at                          time.Time
		props                       map[string]any
	}{
		{"issues_opened", "issues", "issue_opened", repo + "#331", at("2026-10-10T20:31:15Z"),
			map[string]any{"labels": "enhancement"}},
		{"issues_unlabeled_needs_refining", "issues", "issue_refined", repo + "#331", at("2026-10-11T15:43:07Z"), nil},
		{"issues_labeled_needs_you", "issues", "needs_you_raised", repo + "#331", at("2026-10-11T15:43:07Z"), nil},
		{"issues_unlabeled_needs_you", "issues", "needs_you_answered", repo + "#331", at("2026-10-11T15:43:07Z"), nil},
		{"pull_request_opened", "pull_request", "pr_opened", repo + "#331", at("2026-10-11T14:26:10Z"),
			map[string]any{"pr": 339, "branch": "issue-331/user-funnels", "head_sha": "be2627b1f34a0ee4a054a2ad196b1f3908b1308c"}},
		{"pull_request_closed_merged", "pull_request", "pr_merged", repo + "#331", at("2026-10-11T14:58:51Z"),
			map[string]any{"pr": 339, "merge_sha": "c3fceb88e3436d9e64325310bc9635dad4dc5a99"}},
		{"check_suite_completed", "check_suite", "ci_result", repo + "#331", at("2026-10-11T14:58:00Z"),
			map[string]any{"pr": 339, "conclusion": "success", "app": "github-actions", "head_sha": "be2627b1f34a0ee4a054a2ad196b1f3908b1308c"}},
		{"release_published", "release", "release_published", "", at("2026-10-11T15:02:42Z"),
			map[string]any{"tag": "v0.6.84"}},
		{"deployment_status", "deployment_status", "deployment_status", "", at("2026-10-10T09:11:05Z"),
			map[string]any{"environment": "github-pages", "state": "success", "ref": "main", "sha": "f98f7d3fcddf384cc379fdbc39879a1a1d1d4c1f"}},
		{"workflow_run_completed", "workflow_run", "workflow_completed", "", at("2026-10-11T15:36:40Z"),
			map[string]any{"workflow": "Deploy Production", "trigger": "workflow_dispatch", "conclusion": "success", "run_attempt": 1, "first_attempt": true}},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			ev, ok, err := ghwebhook.Map(tc.kind, fixture(t, tc.fixture))
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, tc.name, ev.Name)
			assert.Equal(t, tc.userID, ev.UserID)
			assert.Equal(t, tc.at, ev.Timestamp)
			assert.Equal(t, "production", ev.Environment)
			assert.True(t, session.IsValidSessionID(ev.SessionID), "session id %q", ev.SessionID)
			assert.Equal(t, repo, ev.Properties["repo"])
			assert.Equal(t, ghwebhook.Source, ev.Properties["source"])
			if tc.userID != "" {
				assert.Equal(t, tc.userID, ev.Properties["issue_ref"])
			} else {
				assert.NotContains(t, ev.Properties, "issue_ref")
			}
			for k, want := range tc.props {
				assert.Equal(t, want, ev.Properties[k], "property %s", k)
			}
		})
	}
}

func TestMap_IgnoredDeliveries(t *testing.T) {
	cases := []struct{ fixture, kind string }{
		{"ping", "ping"},
		{"issues_labeled_other", "issues"},
		{"pull_request_closed_unmerged", "pull_request"},
		{"check_suite_completed_no_pr", "check_suite"},
		{"issues_opened", "star"},
	}
	for _, tc := range cases {
		t.Run(tc.fixture+"/"+tc.kind, func(t *testing.T) {
			_, ok, err := ghwebhook.Map(tc.kind, fixture(t, tc.fixture))
			require.NoError(t, err)
			assert.False(t, ok)
		})
	}
}

func TestMap_RejectsMalformedPayload(t *testing.T) {
	_, ok, err := ghwebhook.Map("issues", []byte("{not json"))
	require.Error(t, err)
	assert.False(t, ok)
}

func TestMap_EventsOfOneIssueShareASession(t *testing.T) {
	opened, _, err := ghwebhook.Map("issues", fixture(t, "issues_opened"))
	require.NoError(t, err)
	merged, _, err := ghwebhook.Map("pull_request", fixture(t, "pull_request_closed_merged"))
	require.NoError(t, err)
	release, _, err := ghwebhook.Map("release", fixture(t, "release_published"))
	require.NoError(t, err)
	assert.Equal(t, opened.SessionID, merged.SessionID)
	assert.NotEqual(t, opened.SessionID, release.SessionID)
}

func TestLinkedIssue(t *testing.T) {
	cases := []struct{ name, body, branch, want string }{
		{"closes keyword", "Some text\n\nCloses #12", "feature/x", repo + "#12"},
		{"case and past tense", "this FIXED #7 for good", "", repo + "#7"},
		{"resolves with colon", "Resolves: #44", "", repo + "#44"},
		{"cross-repo ref", "fixes wiebe-xyz/bugbarn#9", "", "wiebe-xyz/bugbarn#9"},
		{"body wins over branch", "Closes #5", "issue-6/thing", repo + "#5"},
		{"branch only", "Part of #331", "issue-331/user-funnels", repo + "#331"},
		{"neither", "Part of #331", "main", ""},
		{"bare mention is no link", "see #8", "", ""},
		{"keyword inside a word", "prefixes #3", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ghwebhook.LinkedIssue(repo, tc.body, tc.branch))
		})
	}
}

func TestPullRequestWithoutLinkHasNoUser(t *testing.T) {
	body := []byte(`{"action":"opened","repository":{"full_name":"webwiebe/funnelbarn"},
		"pull_request":{"number":5,"body":"no link","created_at":"2026-10-01T00:00:00Z","head":{"ref":"chore/deps","sha":"abc"}}}`)
	ev, ok, err := ghwebhook.Map("pull_request", body)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Empty(t, ev.UserID)
	assert.NotContains(t, ev.Properties, "issue_ref")
}

func TestValidSignature(t *testing.T) {
	body := fixture(t, "issues_opened")
	good := ghwebhook.Sign("s3cret", body)
	assert.True(t, ghwebhook.ValidSignature("s3cret", good, body))
	assert.False(t, ghwebhook.ValidSignature("other", good, body), "wrong secret")
	assert.False(t, ghwebhook.ValidSignature("s3cret", good, append([]byte(" "), body...)), "changed body")
	assert.False(t, ghwebhook.ValidSignature("s3cret", "", body), "missing header")
	assert.False(t, ghwebhook.ValidSignature("s3cret", "sha1=abc", body), "wrong algorithm")
	assert.False(t, ghwebhook.ValidSignature("s3cret", "sha256=zz", body), "not hex")
	assert.False(t, ghwebhook.ValidSignature("", ghwebhook.Sign("", body), body), "empty secret never verifies")
}
