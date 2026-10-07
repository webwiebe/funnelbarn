package archtest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Rule defines an architecture constraint.
type Rule struct {
	From          string // Package prefix (e.g., "internal/repository")
	MustNotImport string // Forbidden import prefix
	Reason        string // Why this rule exists
}

// Package represents a parsed Go package from go list -json output.
type Package struct {
	ImportPath string   `json:"ImportPath"`
	Imports    []string `json:"Imports"`
	Standard   bool     `json:"Standard"`
	Goroot     bool     `json:"Goroot"`
}

// Graph holds the import graph and detected violations.
type Graph struct {
	Packages   map[string]*Package
	Violations []*Violation
}

// Violation represents an architecture rule violation.
type Violation struct {
	Rule    *Rule
	Package string
	Import  string
}

// LoadGraph loads the import graph from go list -deps -json.
func LoadGraph(modulePath string) (*Graph, error) {
	// Find go.mod by walking up from modulePath
	goModPath := findGoMod(modulePath)
	if goModPath == "" {
		return nil, fmt.Errorf("go.mod not found from %s", modulePath)
	}

	dir := filepath.Dir(goModPath)
	cmd := exec.Command("go", "list", "-deps", "-json", "./...")
	cmd.Dir = dir
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list failed: %w", err)
	}
	if len(bytes.TrimSpace(output)) == 0 {
		return nil, fmt.Errorf("go list produced no output in %s", dir)
	}

	g := &Graph{
		Packages: make(map[string]*Package),
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	for decoder.More() {
		var pkg Package
		if err := decoder.Decode(&pkg); err != nil {
			return nil, fmt.Errorf("decode package: %w", err)
		}
		// Only keep first-party packages
		if strings.HasPrefix(pkg.ImportPath, "github.com/wiebe-xyz/funnelbarn/") {
			g.Packages[pkg.ImportPath] = &pkg
		}
	}

	if len(g.Packages) == 0 {
		return nil, fmt.Errorf("go list returned no first-party packages in %s", dir)
	}

	return g, nil
}

// Check applies a set of rules and collects violations. It returns an error
// when the graph is empty or when a rule's From pattern matches no package,
// since a rule that matches nothing silently checks nothing.
func (g *Graph) Check(rules []*Rule) error {
	g.Violations = nil
	if len(g.Packages) == 0 {
		return errors.New("archtest: graph has 0 packages")
	}
	matched := make(map[*Rule]bool, len(rules))
	for _, pkg := range g.Packages {
		for _, rule := range rules {
			if !matchesPath(pkg.ImportPath, rule.From) {
				continue
			}
			matched[rule] = true
			for _, imp := range pkg.Imports {
				if matchesPath(imp, rule.MustNotImport) {
					g.Violations = append(g.Violations, &Violation{
						Rule:    rule,
						Package: pkg.ImportPath,
						Import:  imp,
					})
				}
			}
		}
	}
	var unmatched []string
	for _, rule := range rules {
		if !matched[rule] {
			unmatched = append(unmatched, rule.From)
		}
	}
	if len(unmatched) > 0 {
		return fmt.Errorf("archtest: rule From pattern matches no package: %s", strings.Join(unmatched, ", "))
	}
	return nil
}

// matchesPath reports whether importPath is the package named by pattern or
// lies beneath it, on path-segment boundaries. "internal/service" matches
// ".../internal/service" and ".../internal/service/foo" but not
// ".../internal/serviceless".
func matchesPath(importPath, pattern string) bool {
	return importPath == pattern ||
		strings.HasSuffix(importPath, "/"+pattern) ||
		strings.Contains(importPath, "/"+pattern+"/")
}

// findGoMod walks up the directory tree to find go.mod.
func findGoMod(startPath string) string {
	path := startPath
	for {
		candidate := filepath.Join(path, "go.mod")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "" // reached root
		}
		path = parent
	}
}

// Rules returns the architecture rules that must hold.
func Rules() []*Rule {
	return []*Rule{
		{
			From:          "internal/repository",
			MustNotImport: "internal/service",
			Reason:        "repository is a storage adapter; it must not depend on domain services",
		},
		{
			From:          "internal/repository",
			MustNotImport: "internal/api",
			Reason:        "repository is a storage adapter; it must not depend on HTTP handlers",
		},
		{
			From:          "internal/repository",
			MustNotImport: "internal/ports",
			Reason:        "repository is a storage adapter; it must not depend on port interfaces",
		},
		{
			From:          "internal/domain",
			MustNotImport: "internal/repository",
			Reason:        "domain types must not depend on storage implementation",
		},
		{
			From:          "internal/ports",
			MustNotImport: "internal/service",
			Reason:        "ports are interfaces; they must not depend on service implementations",
		},
		{
			From:          "internal/ports",
			MustNotImport: "internal/api",
			Reason:        "ports are interfaces; they must not depend on HTTP handlers",
		},
		{
			From:          "internal/ports",
			MustNotImport: "internal/command",
			Reason:        "ports are interfaces; they must not depend on the command dispatcher",
		},
	}
}
