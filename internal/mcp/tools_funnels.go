package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// registerFunnelTools registers list_funnels and analyze_funnel (read-only),
// and create_funnel, update_funnel and delete_funnel (write). Every tool
// taking a funnel_id loads the funnel with loadFunnel, which enforces the
// package's cross-project rule (see server.go's doc comment).
func registerFunnelTools(s *mcp.Server, d *Deps) {
	addTool(s, d, &mcp.Tool{
		Name: "list_funnels",
		Description: "List a project's funnels (handleListFunnels), each with its steps. A " +
			"funnel's unmatched_steps names any step event the project has never actually " +
			"recorded. Such a funnel was most likely built against the wrong event name and can " +
			"never show a conversion.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, listFunnels)

	addTool(s, d, &mcp.Tool{
		Name: "analyze_funnel",
		Description: "Run funnel conversion analysis over a time range (handleFunnelAnalysis): " +
			"per-step event counts, conversion and drop-off. Defaults to the last 7 days. Pass " +
			"segment_id (see list_segments) to restrict the analysis to sessions matching that " +
			"saved segment's rules; omit it to analyze every session.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, analyzeFunnel)

	addTool(s, d, &mcp.Tool{
		Name: "create_funnel",
		Description: "Create a funnel (handleCreateFunnel): a name, an optional description, " +
			"and an ordered list of steps, each an event name plus optional property filters. " +
			"Call list_event_names first: a step whose event_name the project never actually " +
			"sends can never show a conversion.",
		Annotations: &mcp.ToolAnnotations{OpenWorldHint: ptr(false)},
	}, scopeWrite, createFunnel)

	addTool(s, d, &mcp.Tool{
		Name: "update_funnel",
		Description: "Replace a funnel's name, description and steps (handleUpdateFunnel), " +
			"validated the same way as create_funnel. The funnel's scope (session or page_view) " +
			"is left as is; there is no argument for it here.",
		Annotations: &mcp.ToolAnnotations{OpenWorldHint: ptr(false)},
	}, scopeWrite, updateFunnel)

	addTool(s, d, &mcp.Tool{
		Name:        "delete_funnel",
		Description: "Delete a funnel and its steps (handleDeleteFunnel). This cannot be undone.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: ptr(false)},
	}, scopeWrite, deleteFunnel)
}

// loadFunnel fetches a funnel and checks it belongs to projectID. Skipping
// this check would let a call scoped to one project read, change or delete
// another project's funnel by ID.
func loadFunnel(ctx context.Context, c *Call, projectID, funnelID string) (repository.Funnel, error) {
	f, err := c.Deps.Funnels.GetFunnel(ctx, funnelID)
	if err != nil {
		return repository.Funnel{}, err
	}
	if f.ProjectID != projectID {
		return repository.Funnel{}, fmt.Errorf("%w: funnel %s", domain.ErrNotFound, funnelID)
	}
	return f, nil
}

// funnelStepIn is one step in create_funnel/update_funnel's steps argument:
// only what the caller supplies. step_order is computed from position in the
// list, and id/funnel_id are assigned on write.
type funnelStepIn struct {
	EventName string                    `json:"event_name" jsonschema:"Event name this step matches; see list_event_names."`
	Filters   []repository.FunnelFilter `json:"filters,omitempty" jsonschema:"Optional property filters: the step counts only events where every filter's property equals its value."`
}

func toFunnelSteps(in []funnelStepIn) []repository.FunnelStep {
	steps := make([]repository.FunnelStep, len(in))
	for i, s := range in {
		steps[i] = repository.FunnelStep{EventName: s.EventName, Filters: s.Filters}
	}
	return steps
}

// --- list_funnels ---

type listFunnelsIn struct {
	Project string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
}

type listFunnelsOut struct {
	Project string              `json:"project" jsonschema:"Slug of the project these funnels are from."`
	Funnels []repository.Funnel `json:"funnels" jsonschema:"Every funnel for the project, each with its steps."`
}

func listFunnels(ctx context.Context, c *Call, in listFunnelsIn) (listFunnelsOut, error) {
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return listFunnelsOut{}, err
	}
	list, err := c.Deps.Funnels.ListFunnels(ctx, p.ID)
	if err != nil {
		return listFunnelsOut{}, err
	}
	return listFunnelsOut{Project: p.Slug, Funnels: nonNil(list)}, nil
}

// --- analyze_funnel ---

type analyzeFunnelIn struct {
	Project   string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	FunnelID  string `json:"funnel_id" jsonschema:"Funnel to analyze; see list_funnels."`
	From      string `json:"from,omitempty" jsonschema:"Start of the range, RFC 3339 or YYYY-MM-DD. Defaults to 7 days before to."`
	To        string `json:"to,omitempty" jsonschema:"End of the range, RFC 3339 or YYYY-MM-DD. Defaults to now."`
	SegmentID string `json:"segment_id,omitempty" jsonschema:"Restrict to sessions matching this saved segment's rules; see list_segments. Omit to analyze every session."`
}

type analyzeFunnelOut struct {
	Project string                        `json:"project" jsonschema:"Slug of the project this analysis is for."`
	Funnel  repository.Funnel             `json:"funnel" jsonschema:"The analyzed funnel."`
	From    time.Time                     `json:"from" jsonschema:"Start of the range (inclusive)."`
	To      time.Time                     `json:"to" jsonschema:"End of the range (inclusive)."`
	Results []repository.FunnelStepResult `json:"results" jsonschema:"Per-step count, conversion and drop-off, in step order."`
}

func analyzeFunnel(ctx context.Context, c *Call, in analyzeFunnelIn) (analyzeFunnelOut, error) {
	if in.FunnelID == "" {
		return analyzeFunnelOut{}, invalidInput("funnel_id is required")
	}
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return analyzeFunnelOut{}, err
	}
	funnel, err := loadFunnel(ctx, c, p.ID, in.FunnelID)
	if err != nil {
		return analyzeFunnelOut{}, err
	}
	from, to, err := parseRange(in.From, in.To, time.Now())
	if err != nil {
		return analyzeFunnelOut{}, err
	}

	// A stored segment's rules narrow the analysis the same way
	// handleFunnelAnalysis's segment_id query param does. Unlike that
	// handler (which treats a lookup failure as non-fatal and analyzes
	// without a filter), a segment_id from another project is rejected
	// like any other cross-project object ID rather than silently ignored.
	var rules []repository.SegmentRule
	if in.SegmentID != "" {
		seg, err := loadSegment(ctx, c, p.ID, in.SegmentID)
		if err != nil {
			return analyzeFunnelOut{}, err
		}
		rules = seg.Rules
	}

	results, err := c.Deps.Funnels.AnalyzeFunnel(ctx, funnel, from, to, nil, rules...)
	if err != nil {
		return analyzeFunnelOut{}, err
	}
	return analyzeFunnelOut{
		Project: p.Slug,
		Funnel:  funnel,
		From:    from,
		To:      to,
		Results: nonNil(results),
	}, nil
}

// --- create_funnel ---

type createFunnelIn struct {
	Project     string         `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	Name        string         `json:"name" jsonschema:"Funnel name."`
	Description string         `json:"description,omitempty" jsonschema:"Optional description."`
	Steps       []funnelStepIn `json:"steps" jsonschema:"Ordered funnel steps; at least one is required. Call list_event_names first so event_name matches a real event."`
}

type createFunnelOut struct {
	Funnel repository.Funnel `json:"funnel" jsonschema:"The created funnel."`
}

func createFunnel(ctx context.Context, c *Call, in createFunnelIn) (createFunnelOut, error) {
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return createFunnelOut{}, err
	}
	f, err := c.Deps.Funnels.CreateFunnel(ctx, repository.Funnel{
		ProjectID:   p.ID,
		Name:        in.Name,
		Description: in.Description,
		Steps:       toFunnelSteps(in.Steps),
	})
	if err != nil {
		return createFunnelOut{}, err
	}
	return createFunnelOut{Funnel: f}, nil
}

// --- update_funnel ---

type updateFunnelIn struct {
	Project     string         `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	FunnelID    string         `json:"funnel_id" jsonschema:"Funnel to update; see list_funnels."`
	Name        string         `json:"name" jsonschema:"Funnel name."`
	Description string         `json:"description,omitempty" jsonschema:"Optional description."`
	Steps       []funnelStepIn `json:"steps" jsonschema:"Ordered funnel steps, replacing the existing ones; at least one is required. Call list_event_names first so event_name matches a real event."`
}

type updateFunnelOut struct {
	Funnel repository.Funnel `json:"funnel" jsonschema:"The updated funnel."`
}

func updateFunnel(ctx context.Context, c *Call, in updateFunnelIn) (updateFunnelOut, error) {
	if in.FunnelID == "" {
		return updateFunnelOut{}, invalidInput("funnel_id is required")
	}
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return updateFunnelOut{}, err
	}
	if _, err := loadFunnel(ctx, c, p.ID, in.FunnelID); err != nil {
		return updateFunnelOut{}, err
	}
	f, err := c.Deps.Funnels.UpdateFunnel(ctx, repository.Funnel{
		ID:          in.FunnelID,
		ProjectID:   p.ID,
		Name:        in.Name,
		Description: in.Description,
		Steps:       toFunnelSteps(in.Steps),
	})
	if err != nil {
		return updateFunnelOut{}, err
	}
	return updateFunnelOut{Funnel: f}, nil
}

// --- delete_funnel ---

type deleteFunnelIn struct {
	Project  string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	FunnelID string `json:"funnel_id" jsonschema:"Funnel to delete; see list_funnels. This cannot be undone."`
}

type deleteFunnelOut struct {
	Deleted bool `json:"deleted" jsonschema:"Always true on success."`
}

func deleteFunnel(ctx context.Context, c *Call, in deleteFunnelIn) (deleteFunnelOut, error) {
	if in.FunnelID == "" {
		return deleteFunnelOut{}, invalidInput("funnel_id is required")
	}
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return deleteFunnelOut{}, err
	}
	if _, err := loadFunnel(ctx, c, p.ID, in.FunnelID); err != nil {
		return deleteFunnelOut{}, err
	}
	if err := c.Deps.Funnels.DeleteFunnel(ctx, in.FunnelID); err != nil {
		return deleteFunnelOut{}, err
	}
	return deleteFunnelOut{Deleted: true}, nil
}
