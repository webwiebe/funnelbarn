package mcp

import (
	"context"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// agentsDefaultRange is the agents_lead_times window when from is omitted.
// Issues take days to reach production, so the funnel default of 7 days
// would leave most of them out.
const agentsDefaultRange = 30 * 24 * time.Hour

// registerAgentTools registers agents_scoreboard and agents_lead_times
// (read-only), the delivery report over a project fed by the agent hooks and
// the GitHub webhook (#331).
func registerAgentTools(s *mcp.Server, d *Deps) {
	addTool(s, d, &mcp.Tool{
		Name: "agents_scoreboard",
		Description: "Score the agent sessions of a project over one ISO week (handleAgentScoreboard). " +
			"Points per session per rule: a healthy production deploy +10, a broken one -20, a PR " +
			"merged with every check green on the first run +5, a refined issue that needed no owner " +
			"answer +2, a staging or E2E gate that stopped a bad build +2, a merge on a red build -10. " +
			"Deploys younger than 24 hours are listed under pending. Pass session to see one " +
			"session's entries.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, agentsScoreboard)

	addTool(s, d, &mcp.Tool{
		Name: "agents_lead_times",
		Description: "Delivery lead times over a time range (handleAgentLeadTimes): issue opened to " +
			"first good production deploy, PR opened to merged, owner wait on needs-you questions, " +
			"first-pass CI rate per repository, issues shipped per day and the questions still " +
			"waiting on the owner. Defaults to the last 30 days. Pass repo (owner/name) to narrow " +
			"it to one repository.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, agentsLeadTimes)
}

func agentsService(c *Call) (service.Agents, error) {
	if c.Deps.Agents == nil {
		// A wiring fault: toolError logs it at Error (selflog) and the client
		// sees "internal error", as get_setup_guide does without SetupDoc.
		return nil, errors.New("agents reporting is not configured on this server")
	}
	return c.Deps.Agents, nil
}

// --- agents_scoreboard ---

type agentsScoreboardIn struct {
	Project string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	Week    string `json:"week,omitempty" jsonschema:"ISO week as YYYY-Www, e.g. 2026-W41. Defaults to the current week."`
	Session string `json:"session,omitempty" jsonschema:"Only score this agent session ID."`
}

type agentsScoreboardOut struct {
	Project    string                  `json:"project" jsonschema:"Slug of the project this scoreboard is for."`
	Week       string                  `json:"week" jsonschema:"The scored ISO week."`
	Scoreboard service.AgentScoreboard `json:"scoreboard" jsonschema:"Sessions ranked by points, every scored entry, pending deploys and the rules that could not be scored."`
}

func agentsScoreboard(ctx context.Context, c *Call, in agentsScoreboardIn) (agentsScoreboardOut, error) {
	svc, err := agentsService(c)
	if err != nil {
		return agentsScoreboardOut{}, err
	}
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return agentsScoreboardOut{}, err
	}
	week := in.Week
	if week == "" {
		week = service.CurrentISOWeek(time.Now())
	}
	from, to, err := service.ParseISOWeek(week)
	if err != nil {
		return agentsScoreboardOut{}, err
	}
	board, err := svc.Scoreboard(ctx, p.ID, from, to, in.Session)
	if err != nil {
		return agentsScoreboardOut{}, err
	}
	return agentsScoreboardOut{Project: p.Slug, Week: week, Scoreboard: board}, nil
}

// --- agents_lead_times ---

type agentsLeadTimesIn struct {
	Project string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	From    string `json:"from,omitempty" jsonschema:"Start of the range, RFC 3339 or YYYY-MM-DD. Defaults to 30 days before to."`
	To      string `json:"to,omitempty" jsonschema:"End of the range, RFC 3339 or YYYY-MM-DD. Defaults to now."`
	Repo    string `json:"repo,omitempty" jsonschema:"Only this repository, as owner/name."`
}

type agentsLeadTimesOut struct {
	Project   string                 `json:"project" jsonschema:"Slug of the project this report is for."`
	LeadTimes service.AgentLeadTimes `json:"lead_times" jsonschema:"Median and p90 durations, weekly figures, first-pass CI per repository and per-issue lead times."`
}

func agentsLeadTimes(ctx context.Context, c *Call, in agentsLeadTimesIn) (agentsLeadTimesOut, error) {
	svc, err := agentsService(c)
	if err != nil {
		return agentsLeadTimesOut{}, err
	}
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return agentsLeadTimesOut{}, err
	}
	from, to, err := parseRange(in.From, in.To, time.Now())
	if err != nil {
		return agentsLeadTimesOut{}, err
	}
	if in.From == "" {
		from = to.Add(-agentsDefaultRange)
	}
	out, err := svc.LeadTimes(ctx, p.ID, from, to, in.Repo)
	if err != nil {
		return agentsLeadTimesOut{}, err
	}
	return agentsLeadTimesOut{Project: p.Slug, LeadTimes: out}, nil
}
