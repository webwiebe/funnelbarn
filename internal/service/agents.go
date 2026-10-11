package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
)

const (
	// agentLookback is how far before a range the service loads events, so an
	// issue shipped in the range still finds the issue_opened it started with.
	agentLookback = 90 * 24 * time.Hour
	// agentHealthWindow is how long a production deploy must stay healthy.
	agentHealthWindow = 24 * time.Hour
	// agentDeployAttribution bounds how long before a deploy finished the
	// session's prod_deploy_started may have fired.
	agentDeployAttribution = 3 * time.Hour
	// agentAttributionSlack tolerates a hook event that lands just after the
	// GitHub event it belongs to.
	agentAttributionSlack = 10 * time.Minute
	// agentMaxRange caps a requested range.
	agentMaxRange = 400 * 24 * time.Hour
)

// AgentService computes the agent fleet's delivery lead times and the
// per-session scoreboard from the agents project's events (#331, #333).
type AgentService struct {
	q   ports.AgentQueries
	now func() time.Time
}

// NewAgentService creates an AgentService.
func NewAgentService(q ports.AgentQueries) *AgentService {
	return &AgentService{q: q, now: time.Now}
}

// WithClock replaces the service's clock, for tests.
func (s *AgentService) WithClock(now func() time.Time) *AgentService {
	s.now = now
	return s
}

// DurationStats summarises a set of durations, in whole seconds.
type DurationStats struct {
	Count         int   `json:"count"`
	MedianSeconds int64 `json:"median_seconds"`
	P90Seconds    int64 `json:"p90_seconds"`
}

// AgentWeekStats is the issue-to-production lead time of one ISO week.
type AgentWeekStats struct {
	Week string `json:"week"`
	DurationStats
}

// AgentRepoCI is the first-pass CI rate of one repository.
type AgentRepoCI struct {
	Repo           string  `json:"repo"`
	PRs            int     `json:"prs"`
	FirstPassGreen int     `json:"first_pass_green"`
	Rate           float64 `json:"rate"`
}

// AgentOpenWait is a needs-you question still waiting for the owner.
type AgentOpenWait struct {
	IssueRef       string    `json:"issue_ref"`
	Since          time.Time `json:"since"`
	WaitingSeconds int64     `json:"waiting_seconds"`
}

// AgentDayCount counts issues shipped on one UTC day (YYYY-MM-DD).
type AgentDayCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// AgentIssueLead is one shipped issue.
type AgentIssueLead struct {
	IssueRef    string     `json:"issue_ref"`
	Repo        string     `json:"repo"`
	OpenedAt    *time.Time `json:"opened_at,omitempty"`
	MergedAt    time.Time  `json:"merged_at"`
	ShippedAt   time.Time  `json:"shipped_at"`
	LeadSeconds *int64     `json:"lead_seconds,omitempty"`
}

// AgentLeadTimes is the delivery report for a range.
type AgentLeadTimes struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
	Repo string    `json:"repo,omitempty"`
	// IssueToProd runs from issue_opened to the first good production deploy
	// of the repo after the issue's first merged PR, for issues shipped in
	// the range.
	IssueToProd DurationStats `json:"issue_to_prod"`
	// PRToMerge covers pull requests merged in the range.
	PRToMerge DurationStats `json:"pr_to_merge"`
	// OwnerWait covers needs-you questions answered in the range.
	OwnerWait     DurationStats    `json:"owner_wait"`
	Weekly        []AgentWeekStats `json:"weekly"`
	FirstPassCI   []AgentRepoCI    `json:"first_pass_ci"`
	OpenWaits     []AgentOpenWait  `json:"open_waits"`
	ShippedPerDay []AgentDayCount  `json:"shipped_per_day"`
	Issues        []AgentIssueLead `json:"issues"`
}

func (s *AgentService) load(ctx context.Context, projectID string, from, to time.Time) (*agentModel, error) {
	if !to.After(from) {
		return nil, &domain.ValidationError{Field: "to", Message: "must be after from"}
	}
	if to.Sub(from) > agentMaxRange {
		return nil, &domain.ValidationError{Field: "from", Message: "range is longer than 400 days"}
	}
	events, err := s.q.AgentEvents(ctx, projectID, agentEventNames, from.Add(-agentLookback), to.Add(agentHealthWindow))
	if err != nil {
		return nil, fmt.Errorf("load agent events: %w", err)
	}
	return buildAgentModel(events), nil
}

// LeadTimes reports issue-to-production and PR-to-merge lead times, the
// first-pass CI rate per repo, owner wait times and issues shipped per day.
// repo, when set, limits the report to one repository (owner/name).
func (s *AgentService) LeadTimes(ctx context.Context, projectID string, from, to time.Time, repo string) (AgentLeadTimes, error) {
	m, err := s.load(ctx, projectID, from, to)
	if err != nil {
		return AgentLeadTimes{}, err
	}
	in := func(t time.Time) bool { return !t.Before(from) && !t.After(to) }
	out := AgentLeadTimes{From: from, To: to, Repo: repo}

	var leads []time.Duration
	weekly := map[string][]time.Duration{}
	perDay := map[string]int{}
	for _, iss := range m.issues {
		if (repo != "" && iss.repo != repo) || !in(iss.shippedAt) {
			continue
		}
		merged := iss.firstMerge()
		row := AgentIssueLead{IssueRef: iss.ref, Repo: iss.repo, MergedAt: merged.mergedAt, ShippedAt: iss.shippedAt}
		perDay[iss.shippedAt.UTC().Format(time.DateOnly)]++
		if !iss.openedAt.IsZero() {
			opened := iss.openedAt
			d := iss.shippedAt.Sub(opened)
			secs := int64(d / time.Second)
			row.OpenedAt, row.LeadSeconds = &opened, &secs
			leads = append(leads, d)
			week := isoWeek(iss.shippedAt)
			weekly[week] = append(weekly[week], d)
		}
		out.Issues = append(out.Issues, row)
	}
	out.IssueToProd = durationStats(leads)
	out.Weekly = weeklyStats(weekly)
	out.ShippedPerDay = dayCounts(perDay)
	sort.Slice(out.Issues, func(i, j int) bool { return out.Issues[i].ShippedAt.Before(out.Issues[j].ShippedAt) })

	out.PRToMerge, out.FirstPassCI = m.prStats(in, repo)
	out.OwnerWait, out.OpenWaits = m.ownerWaits(from, to, repo, s.now())
	out.Issues, out.Weekly = nonNilSlice(out.Issues), nonNilSlice(out.Weekly)
	return out, nil
}

func (m *agentModel) prStats(in func(time.Time) bool, repo string) (DurationStats, []AgentRepoCI) {
	var toMerge []time.Duration
	perRepo := map[string]*AgentRepoCI{}
	for _, pr := range m.prs {
		if (repo != "" && pr.repo != repo) || !in(pr.mergedAt) {
			continue
		}
		if !pr.openedAt.IsZero() {
			toMerge = append(toMerge, pr.mergedAt.Sub(pr.openedAt))
		}
		first, ok := m.firstCI(pr)
		if !ok {
			continue
		}
		rc := perRepo[pr.repo]
		if rc == nil {
			rc = &AgentRepoCI{Repo: pr.repo}
			perRepo[pr.repo] = rc
		}
		rc.PRs++
		if first.conclusion == "success" {
			rc.FirstPassGreen++
		}
	}
	rates := []AgentRepoCI{}
	for _, rc := range perRepo {
		rc.Rate = float64(rc.FirstPassGreen) / float64(rc.PRs)
		rates = append(rates, *rc)
	}
	sort.Slice(rates, func(i, j int) bool { return rates[i].Repo < rates[j].Repo })
	return durationStats(toMerge), rates
}

// ownerWaits pairs each needs_you_raised with the next needs_you_answered
// of the same issue. Answered pairs in [from, to] feed the stats; a raise
// with no answer by to is an open wait, measured up to min(to, now).
func (m *agentModel) ownerWaits(from, to time.Time, repo string, now time.Time) (DurationStats, []AgentOpenWait) {
	end := to
	if now.Before(end) {
		end = now
	}
	var waits []time.Duration
	open := []AgentOpenWait{}
	for _, iss := range m.issues {
		if repo != "" && iss.repo != repo {
			continue
		}
		for _, raised := range iss.raised {
			answered, ok := firstAfter(iss.answered, raised)
			switch {
			case ok && !answered.Before(from) && !answered.After(to):
				waits = append(waits, answered.Sub(raised))
			case (!ok || answered.After(to)) && !raised.After(to):
				open = append(open, AgentOpenWait{IssueRef: iss.ref, Since: raised, WaitingSeconds: int64(end.Sub(raised) / time.Second)})
			}
		}
	}
	sort.Slice(open, func(i, j int) bool { return open[i].Since.Before(open[j].Since) })
	return durationStats(waits), open
}

func firstAfter(ts []time.Time, after time.Time) (time.Time, bool) {
	var best time.Time
	found := false
	for _, t := range ts {
		if t.Before(after) {
			continue
		}
		if !found || t.Before(best) {
			best, found = t, true
		}
	}
	return best, found
}

func durationStats(ds []time.Duration) DurationStats {
	if len(ds) == 0 {
		return DurationStats{}
	}
	sorted := append([]time.Duration(nil), ds...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return DurationStats{
		Count:         len(sorted),
		MedianSeconds: int64(nearestRank(sorted, 0.5) / time.Second),
		P90Seconds:    int64(nearestRank(sorted, 0.9) / time.Second),
	}
}

// nearestRank is the nearest-rank percentile p of a sorted, non-empty slice.
func nearestRank(sorted []time.Duration, p float64) time.Duration {
	idx := int(p*float64(len(sorted))+0.999999) - 1
	if idx < 0 {
		idx = 0
	}
	return sorted[idx]
}

func weeklyStats(byWeek map[string][]time.Duration) []AgentWeekStats {
	out := make([]AgentWeekStats, 0, len(byWeek))
	for week, ds := range byWeek {
		out = append(out, AgentWeekStats{Week: week, DurationStats: durationStats(ds)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Week < out[j].Week })
	return out
}

func dayCounts(byDay map[string]int) []AgentDayCount {
	out := make([]AgentDayCount, 0, len(byDay))
	for day, n := range byDay {
		out = append(out, AgentDayCount{Date: day, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// isoWeek formats t's ISO 8601 week as YYYY-Www.
func isoWeek(t time.Time) string {
	y, w := t.UTC().ISOWeek()
	return fmt.Sprintf("%04d-W%02d", y, w)
}

// ParseISOWeek returns the range of an ISO 8601 week written YYYY-Www:
// Monday 00:00 UTC up to the last nanosecond of Sunday.
func ParseISOWeek(s string) (from, to time.Time, err error) {
	year, week, ok := strings.Cut(strings.ToUpper(strings.TrimSpace(s)), "-W")
	y, errY := strconv.Atoi(year)
	w, errW := strconv.Atoi(week)
	if !ok || errY != nil || errW != nil || w < 1 || w > 53 || y < 2000 || y > 9999 {
		return time.Time{}, time.Time{}, &domain.ValidationError{Field: "week", Message: "must look like 2026-W41"}
	}
	// January 4th is always in week 1.
	jan4 := time.Date(y, time.January, 4, 0, 0, 0, 0, time.UTC)
	monday := jan4.AddDate(0, 0, -((int(jan4.Weekday())+6)%7)+(w-1)*7)
	if _, got := monday.ISOWeek(); got != w {
		return time.Time{}, time.Time{}, &domain.ValidationError{Field: "week", Message: fmt.Sprintf("%d has no week %d", y, w)}
	}
	return monday, monday.AddDate(0, 0, 7).Add(-time.Nanosecond), nil
}

// CurrentISOWeek is the ISO week containing now, as YYYY-Www.
func CurrentISOWeek(now time.Time) string { return isoWeek(now) }
