package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// segmentRuleOperators are the operators repository.SegmentRule accepts (see
// service.validateRules). Named here so the create/update tool descriptions
// and validation error can never drift apart.
var segmentRuleOperators = []string{"eq", "neq", "contains", "not_contains", "is_null", "is_not_null"}

// allowedSegmentFieldNames lists the segment rule fields the store accepts,
// sorted for a stable tool description. Read from
// repository.AllowedSegmentFields rather than duplicated here, so a new
// field is picked up without touching this file.
func allowedSegmentFieldNames() []string {
	names := make([]string, 0, len(repository.AllowedSegmentFields))
	for f := range repository.AllowedSegmentFields {
		names = append(names, f)
	}
	sort.Strings(names)
	return names
}

// segmentRuleDoc describes a segment rule's shape and allowed values, shared
// by create_segment and update_segment's descriptions.
func segmentRuleDoc() string {
	return fmt.Sprintf(
		"Each rule is {field, operator, value}: field is one of %s; operator is one of %s "+
			"(value is ignored for is_null/is_not_null).",
		strings.Join(allowedSegmentFieldNames(), ", "),
		strings.Join(segmentRuleOperators, ", "),
	)
}

// registerSegmentTools registers list_segments (read-only), and
// create_segment, update_segment and delete_segment (write). Every tool
// taking a segment_id loads the segment with loadSegment, which enforces the
// package's cross-project rule (see server.go's doc comment).
func registerSegmentTools(s *mcp.Server, d *Deps) {
	addTool(s, d, &mcp.Tool{
		Name: "list_segments",
		Description: "List a project's saved segments (handleListSegments), each with its " +
			"filter rules. Pass a segment's id as analyze_funnel's segment_id argument.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, listSegments)

	addTool(s, d, &mcp.Tool{
		Name: "create_segment",
		Description: "Create a saved segment (handleCreateSegment): a name and a list of filter " +
			"rules. " + segmentRuleDoc() + " Call list_event_properties/list_event_property_values " +
			"first so a rule's field and value match real data.",
		Annotations: &mcp.ToolAnnotations{OpenWorldHint: ptr(false)},
	}, scopeWrite, createSegment)

	addTool(s, d, &mcp.Tool{
		Name: "update_segment",
		Description: "Replace a saved segment's name and rules (handleUpdateSegment). " +
			segmentRuleDoc(),
		Annotations: &mcp.ToolAnnotations{OpenWorldHint: ptr(false)},
	}, scopeWrite, updateSegment)

	addTool(s, d, &mcp.Tool{
		Name: "delete_segment",
		Description: "Delete a saved segment (handleDeleteSegment). A funnel analysis that " +
			"referenced this segment_id will error instead of silently analyzing every session. " +
			"This cannot be undone.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: ptr(false)},
	}, scopeWrite, deleteSegment)
}

// loadSegment fetches a segment and checks it belongs to projectID. Skipping
// this check would let a call scoped to one project read, change or delete
// another project's segment by ID.
func loadSegment(ctx context.Context, c *Call, projectID, segmentID string) (repository.Segment, error) {
	seg, err := c.Deps.Segments.GetSegment(ctx, segmentID)
	if err != nil {
		return repository.Segment{}, err
	}
	if seg.ProjectID != projectID {
		return repository.Segment{}, fmt.Errorf("%w: segment %s", domain.ErrNotFound, segmentID)
	}
	return seg, nil
}

// --- list_segments ---

type listSegmentsIn struct {
	Project string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
}

type listSegmentsOut struct {
	Project  string               `json:"project" jsonschema:"Slug of the project these segments are from."`
	Segments []repository.Segment `json:"segments" jsonschema:"Every saved segment for the project."`
}

func listSegments(ctx context.Context, c *Call, in listSegmentsIn) (listSegmentsOut, error) {
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return listSegmentsOut{}, err
	}
	list, err := c.Deps.Segments.ListSegments(ctx, p.ID)
	if err != nil {
		return listSegmentsOut{}, err
	}
	return listSegmentsOut{Project: p.Slug, Segments: nonNil(list)}, nil
}

// --- create_segment ---

type createSegmentIn struct {
	Project string                   `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	Name    string                   `json:"name" jsonschema:"Segment name."`
	Rules   []repository.SegmentRule `json:"rules,omitempty" jsonschema:"Filter rules; see the tool description for allowed field and operator values. A segment with no rules matches every session."`
}

type createSegmentOut struct {
	Segment repository.Segment `json:"segment" jsonschema:"The created segment."`
}

func createSegment(ctx context.Context, c *Call, in createSegmentIn) (createSegmentOut, error) {
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return createSegmentOut{}, err
	}
	seg, err := c.Deps.Segments.CreateSegment(ctx, p.ID, in.Name, in.Rules)
	if err != nil {
		return createSegmentOut{}, err
	}
	return createSegmentOut{Segment: seg}, nil
}

// --- update_segment ---

type updateSegmentIn struct {
	Project   string                   `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	SegmentID string                   `json:"segment_id" jsonschema:"Segment to update; see list_segments."`
	Name      string                   `json:"name" jsonschema:"Segment name."`
	Rules     []repository.SegmentRule `json:"rules,omitempty" jsonschema:"Filter rules, replacing the existing ones; see the tool description for allowed field and operator values. A segment with no rules matches every session."`
}

type updateSegmentOut struct {
	Segment repository.Segment `json:"segment" jsonschema:"The updated segment."`
}

func updateSegment(ctx context.Context, c *Call, in updateSegmentIn) (updateSegmentOut, error) {
	if in.SegmentID == "" {
		return updateSegmentOut{}, invalidInput("segment_id is required")
	}
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return updateSegmentOut{}, err
	}
	if _, err := loadSegment(ctx, c, p.ID, in.SegmentID); err != nil {
		return updateSegmentOut{}, err
	}
	seg, err := c.Deps.Segments.UpdateSegment(ctx, in.SegmentID, in.Name, in.Rules)
	if err != nil {
		return updateSegmentOut{}, err
	}
	return updateSegmentOut{Segment: seg}, nil
}

// --- delete_segment ---

type deleteSegmentIn struct {
	Project   string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	SegmentID string `json:"segment_id" jsonschema:"Segment to delete; see list_segments. This cannot be undone."`
}

type deleteSegmentOut struct {
	Deleted bool `json:"deleted" jsonschema:"Always true on success."`
}

func deleteSegment(ctx context.Context, c *Call, in deleteSegmentIn) (deleteSegmentOut, error) {
	if in.SegmentID == "" {
		return deleteSegmentOut{}, invalidInput("segment_id is required")
	}
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return deleteSegmentOut{}, err
	}
	if _, err := loadSegment(ctx, c, p.ID, in.SegmentID); err != nil {
		return deleteSegmentOut{}, err
	}
	if err := c.Deps.Segments.DeleteSegment(ctx, in.SegmentID); err != nil {
		return deleteSegmentOut{}, err
	}
	return deleteSegmentOut{Deleted: true}, nil
}
