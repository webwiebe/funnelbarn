package api

import (
	"strings"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// With MCP enabled, the guide opens with a pitch to the reading assistant,
// and every install command works without the project header.
func TestBuildSetupMarkdown_MCPEnabled(t *testing.T) {
	doc := buildSetupMarkdown(repository.Project{Slug: "shop", Status: "active"}, "key", "https://fb.example", 10, true)

	callout := strings.Index(doc, "## Using an AI assistant? Connect the FunnelBarn MCP server first")
	config := strings.Index(doc, "## Project Configuration")
	if callout < 0 || config < 0 || callout > config {
		t.Fatalf("MCP callout must come before Project Configuration (callout=%d, config=%d)", callout, config)
	}

	plain := "claude mcp add --transport http funnelbarn https://fb.example/api/v1/mcp\n"
	if n := strings.Count(doc, plain); n < 2 {
		t.Errorf("want the header-less add command in the callout and the MCP section, found %d", n)
	}
	if !strings.Contains(doc, `{"mcpServers":{"funnelbarn":{"type":"http","url":"https://fb.example/api/v1/mcp"}}}`) {
		t.Error(".mcp.json snippet should carry no headers")
	}
	if !strings.Contains(doc, `"project": "shop"`) {
		t.Error("callout should tell the assistant which project argument to pass")
	}
	if !strings.Contains(doc, "Optional: pin a default project") {
		t.Error("the header should still be documented, as optional")
	}
	if !strings.Contains(doc, "1. Connect the FunnelBarn MCP server") {
		t.Error("next steps should start with connecting the MCP server")
	}
	if !strings.Contains(doc, "`create_funnel` MCP tool") {
		t.Error("recommended funnels should point at create_funnel")
	}
}

// Without MCP the guide never mentions it, so it never sends an assistant to
// a server the instance does not run.
func TestBuildSetupMarkdown_MCPDisabled(t *testing.T) {
	doc := buildSetupMarkdown(repository.Project{Slug: "shop", Status: "active"}, "key", "https://fb.example", 10, false)

	for _, s := range []string{"MCP", "mcp add", "create_funnel"} {
		if strings.Contains(doc, s) {
			t.Errorf("setup guide mentions %q with MCP disabled", s)
		}
	}
	if !strings.Contains(doc, "1. Instrument your app using the SDK examples above") {
		t.Error("next steps should start with instrumenting when MCP is off")
	}
}
