package archtest_test

import (
	"os"
	"strings"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/archtest"
)

// TestArchitecture verifies that the actual codebase obeys all architecture rules.
func TestArchitecture(t *testing.T) {
	pwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	g, err := archtest.LoadGraph(pwd)
	if err != nil {
		t.Fatalf("LoadGraph: %v", err)
	}

	if err := g.Check(archtest.Rules()); err != nil {
		t.Fatalf("Check: %v", err)
	}

	if len(g.Violations) > 0 {
		var buf strings.Builder
		buf.WriteString("architecture violations detected:\n")
		for _, v := range g.Violations {
			buf.WriteString("\n")
			buf.WriteString(v.Package + " imports " + v.Import + "\n")
			buf.WriteString("  VIOLATES: " + v.Rule.From + " must not import " + v.Rule.MustNotImport + "\n")
			buf.WriteString("  REASON: " + v.Rule.Reason + "\n")
		}
		t.Fatal(buf.String())
	}
}

// TestRuleEngine verifies that violations are detected correctly using a synthetic graph.
func TestRuleEngine(t *testing.T) {
	rules := []*archtest.Rule{
		{
			From:          "module/storage",
			MustNotImport: "module/service",
			Reason:        "storage must not depend on services",
		},
	}

	g := &archtest.Graph{
		Packages: map[string]*archtest.Package{
			"github.com/example/app/module/storage": {
				ImportPath: "github.com/example/app/module/storage",
				Imports: []string{
					"github.com/example/app/module/domain",
					"github.com/example/app/module/service", // violation
				},
			},
			"github.com/example/app/module/service": {
				ImportPath: "github.com/example/app/module/service",
				Imports: []string{
					"github.com/example/app/module/domain",
				},
			},
		},
	}

	if err := g.Check(rules); err != nil {
		t.Fatalf("Check: %v", err)
	}

	if len(g.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(g.Violations))
	}

	v := g.Violations[0]
	if v.Package != "github.com/example/app/module/storage" {
		t.Errorf("wrong package: %s", v.Package)
	}
	if v.Import != "github.com/example/app/module/service" {
		t.Errorf("wrong import: %s", v.Import)
	}
	if v.Rule.MustNotImport != "module/service" {
		t.Errorf("wrong rule: %s", v.Rule.MustNotImport)
	}
}

// TestRuleEngine_NoViolations verifies that a clean graph passes.
func TestRuleEngine_NoViolations(t *testing.T) {
	rules := []*archtest.Rule{
		{
			From:          "module/storage",
			MustNotImport: "module/service",
			Reason:        "storage must not depend on services",
		},
	}

	g := &archtest.Graph{
		Packages: map[string]*archtest.Package{
			"github.com/example/app/module/storage": {
				ImportPath: "github.com/example/app/module/storage",
				Imports: []string{
					"github.com/example/app/module/domain",
				},
			},
			"github.com/example/app/module/service": {
				ImportPath: "github.com/example/app/module/service",
				Imports: []string{
					"github.com/example/app/module/domain",
				},
			},
		},
	}

	if err := g.Check(rules); err != nil {
		t.Fatalf("Check: %v", err)
	}

	if len(g.Violations) != 0 {
		t.Fatalf("expected 0 violations, got %d", len(g.Violations))
	}
}

func pkgs(m map[string][]string) *archtest.Graph {
	g := &archtest.Graph{Packages: map[string]*archtest.Package{}}
	for p, imps := range m {
		g.Packages[p] = &archtest.Package{ImportPath: p, Imports: imps}
	}
	return g
}

// TestRuleEngine_Subpackages verifies From and MustNotImport cover subpackages
// but not siblings that merely share a name prefix.
func TestRuleEngine_Subpackages(t *testing.T) {
	rules := []*archtest.Rule{{From: "internal/repository", MustNotImport: "internal/service", Reason: "r"}}
	g := pkgs(map[string][]string{
		"m/internal/repository/sqlcgen": {"m/internal/service/sub"},
		"m/internal/repository/mock":    {"m/internal/domain"},
		"m/internal/repositoryx":        {"m/internal/service"},
		"m/internal/repository":         {"m/internal/serviceless"},
		"m/internal/service/sub":        nil,
		"m/internal/serviceless":        nil,
	})
	if err := g.Check(rules); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(g.Violations) != 1 || g.Violations[0].Package != "m/internal/repository/sqlcgen" {
		t.Fatalf("want exactly the sqlcgen violation, got %d: %+v", len(g.Violations), g.Violations)
	}
}

func TestRuleEngine_EmptyGraphFails(t *testing.T) {
	g := &archtest.Graph{Packages: map[string]*archtest.Package{}}
	if err := g.Check([]*archtest.Rule{{From: "a", MustNotImport: "b"}}); err == nil {
		t.Fatal("expected error for empty graph")
	}
}

func TestRuleEngine_UnmatchedFromFails(t *testing.T) {
	g := pkgs(map[string][]string{"m/internal/domain": nil})
	err := g.Check([]*archtest.Rule{{From: "internal/repository", MustNotImport: "internal/service"}})
	if err == nil || !strings.Contains(err.Error(), "internal/repository") {
		t.Fatalf("expected unmatched From error naming the pattern, got %v", err)
	}
}
