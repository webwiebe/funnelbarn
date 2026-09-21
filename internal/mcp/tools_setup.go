package mcp

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerSetupTools registers get_setup_guide.
func registerSetupTools(s *mcp.Server, d *Deps) {
	addTool(s, d, &mcp.Tool{
		Name: "get_setup_guide",
		Description: "Get the project's setup guide: the same Markdown document served at " +
			"GET /api/v1/setup/{slug}, with SDK install snippets, required headers, the event " +
			"body schema and feature-flag evaluation. Defaults to the repository's project " +
			"(the x-funnelbarn-project header).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, getSetupGuide)
}

type getSetupGuideIn struct {
	Project string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
}

type getSetupGuideOut struct {
	Markdown string `json:"markdown" jsonschema:"The setup guide, as Markdown."`
}

func getSetupGuide(ctx context.Context, c *Call, in getSetupGuideIn) (getSetupGuideOut, error) {
	if c.Deps.SetupDoc == nil {
		// Not an argument problem: it's this server wired without a setup
		// renderer. toolError logs it at Error (selflog) and the client sees
		// "internal error", same as any other unexpected failure.
		return getSetupGuideOut{}, errors.New("setup guide is not configured on this server")
	}
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return getSetupGuideOut{}, err
	}
	doc, err := c.Deps.SetupDoc(ctx, p)
	if err != nil {
		return getSetupGuideOut{}, err
	}
	return getSetupGuideOut{Markdown: doc}, nil
}
