package mcp

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerProjectTools registers list_projects and get_project.
func registerProjectTools(s *mcp.Server, d *Deps) {
	addTool(s, d, &mcp.Tool{
		Name: "list_projects",
		Description: "List every FunnelBarn project the caller can reach, with each project's " +
			"slug and ID. Use this to find the slug to pass as a project argument, or to see " +
			"which project this repository defaults to.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, listProjects)

	addTool(s, d, &mcp.Tool{
		Name: "get_project",
		Description: "Get a project's details plus its integration health: whether the SDK's " +
			"setup call, events, flag evaluations and session recordings have actually been " +
			"seen. Defaults to the repository's project (the x-funnelbarn-project header).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, getProject)
}

// projectSummary is a project as listed by list_projects.
type projectSummary struct {
	ID     string `json:"id" jsonschema:"Project ID."`
	Name   string `json:"name" jsonschema:"Project name."`
	Slug   string `json:"slug" jsonschema:"Project slug, the short name used in the project argument and the x-funnelbarn-project header."`
	Domain string `json:"domain" jsonschema:"The project's configured domain, empty if unset."`
	Status string `json:"status" jsonschema:"Project status, e.g. pending or active."`
}

type listProjectsIn struct{}

type listProjectsOut struct {
	Projects []projectSummary `json:"projects" jsonschema:"Every project the caller can reach."`
	Default  string           `json:"default" jsonschema:"Slug of the project the x-funnelbarn-project header resolves to; empty if the header is unset or does not resolve to a project."`
}

func listProjects(ctx context.Context, c *Call, _ listProjectsIn) (listProjectsOut, error) {
	list, err := c.Deps.Projects.ListProjects(ctx)
	if err != nil {
		return listProjectsOut{}, err
	}
	out := listProjectsOut{Projects: make([]projectSummary, len(list))}
	for i, p := range list {
		out.Projects[i] = projectSummary{ID: p.ID, Name: p.Name, Slug: p.Slug, Domain: p.Domain, Status: p.Status}
	}
	if header := c.Header.Get(ProjectHeader); header != "" {
		if p, err := ResolveProject(ctx, c.Deps.Projects, "", header); err == nil {
			out.Default = p.Slug
		}
	}
	return out, nil
}

type getProjectIn struct {
	Project string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
}

// projectHealthOut mirrors repository.ProjectHealth's flags without the
// bookkeeping fields (project ID, updated-at).
type projectHealthOut struct {
	SetupCalled        bool `json:"setup_called" jsonschema:"Whether the project's SDK setup call has been received."`
	EventsReceived     bool `json:"events_received" jsonschema:"Whether at least one event has been ingested."`
	FlagsEvaluated     bool `json:"flags_evaluated" jsonschema:"Whether a feature flag has been evaluated."`
	RecordingsReceived bool `json:"recordings_received" jsonschema:"Whether a session recording chunk has been received."`
}

type getProjectOut struct {
	ID        string           `json:"id" jsonschema:"Project ID."`
	Name      string           `json:"name" jsonschema:"Project name."`
	Slug      string           `json:"slug" jsonschema:"Project slug."`
	Domain    string           `json:"domain" jsonschema:"The project's configured domain, empty if unset."`
	Status    string           `json:"status" jsonschema:"Project status, e.g. pending or active."`
	CreatedAt time.Time        `json:"created_at" jsonschema:"When the project was created."`
	Health    projectHealthOut `json:"health" jsonschema:"Integration health: which SDK calls this project has actually received, not just whether it is configured to send them."`
}

func getProject(ctx context.Context, c *Call, in getProjectIn) (getProjectOut, error) {
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return getProjectOut{}, err
	}
	h, err := c.Deps.ProjectHealth.GetProjectHealth(ctx, p.ID)
	if err != nil {
		return getProjectOut{}, err
	}
	return getProjectOut{
		ID:        p.ID,
		Name:      p.Name,
		Slug:      p.Slug,
		Domain:    p.Domain,
		Status:    p.Status,
		CreatedAt: p.CreatedAt,
		Health: projectHealthOut{
			SetupCalled:        h.SetupCalled,
			EventsReceived:     h.EventsReceived,
			FlagsEvaluated:     h.FlagsEvaluated,
			RecordingsReceived: h.RecordingsReceived,
		},
	}, nil
}
