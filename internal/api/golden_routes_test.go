package api

// Route enumeration for the golden API suite. The route list is derived from
// the source of the package (every Handle / HandleFunc call on any receiver),
// so a route added later shows up here without anyone editing a list.
// TestGoldenRouteCoverage then fails until the new route, whatever its method,
// has a golden case or an explicit, justified skip.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// goldenRoute is one "METHOD /path" mux registration; Method "*" is a
// registration without a method, which answers every method.
type goldenRoute struct {
	Method  string
	Pattern string // path with {wildcards}
}

func (r goldenRoute) String() string { return r.Method + " " + r.Pattern }

// wildcards counts the {name} segments; fewer wildcards means more specific,
// which is how net/http's mux ranks overlapping patterns.
func (r goldenRoute) wildcards() int { return strings.Count(r.Pattern, "{") }

// regexp turns the pattern into a matcher for concrete request paths.
func (r goldenRoute) regexp() *regexp.Regexp {
	re := regexp.QuoteMeta(r.Pattern)
	re = regexp.MustCompile(`\\\{[a-z_]+\\\}`).ReplaceAllString(re, `[^/]+`)
	return regexp.MustCompile("^" + re + "$")
}

// goldenParseRoutes parses every non-test file of the package, resolves string
// constants (e.g. themeManifestPath, mcpPath), and returns every Handle and
// HandleFunc registration on any receiver, sorted. A pattern without a METHOD
// prefix (the MCP routes) answers every method and is recorded as method "*".
// A call whose pattern cannot be resolved fails the test: silently skipping it
// would let a route escape the coverage check.
func goldenParseRoutes(t *testing.T) []goldenRoute {
	t.Helper()
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	consts := map[string]string{}
	var parsed []*ast.File
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		parsed = append(parsed, f)
	}
	// Two passes so a constant may reference one declared in a later file.
	for i := 0; i < 2; i++ {
		for _, f := range parsed {
			collectStringConsts(f, consts)
		}
	}

	var routes []goldenRoute
	for _, f := range parsed {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc") {
				return true
			}
			if len(call.Args) == 0 {
				t.Fatalf("%s: %s call without a pattern", fset.Position(call.Pos()), sel.Sel.Name)
			}
			pattern, ok := evalStringExpr(call.Args[0], consts)
			if !ok {
				t.Fatalf("%s: cannot resolve route pattern of %s call", fset.Position(call.Pos()), sel.Sel.Name)
			}
			method, path, found := strings.Cut(pattern, " ")
			if !found {
				method, path = "*", pattern
			}
			routes = append(routes, goldenRoute{Method: method, Pattern: path})
			return true
		})
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].String() < routes[j].String() })
	if len(routes) < 50 {
		t.Fatalf("route parser found only %d routes; the parser is broken", len(routes))
	}
	return routes
}

func collectStringConsts(f *ast.File, out map[string]string) {
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				if v, ok := evalStringExpr(vs.Values[i], out); ok {
					out[name.Name] = v
				}
			}
		}
	}
}

// evalStringExpr evaluates string literals, identifiers naming known string
// constants, and "+" concatenations of those.
func evalStringExpr(e ast.Expr, consts map[string]string) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(v.Value)
		return s, err == nil
	case *ast.Ident:
		s, ok := consts[v.Name]
		return s, ok
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", false
		}
		l, lok := evalStringExpr(v.X, consts)
		r, rok := evalStringExpr(v.Y, consts)
		return l + r, lok && rok
	case *ast.ParenExpr:
		return evalStringExpr(v.X, consts)
	}
	return "", false
}

// goldenMatchRoute returns the most specific route of the given method whose
// pattern matches path, mirroring the mux's precedence for literal-vs-wildcard
// siblings such as /flags/context-keys and /flags/{fid}.
func goldenMatchRoute(routes []goldenRoute, method, path string) (goldenRoute, bool) {
	var best goldenRoute
	found := false
	for _, r := range routes {
		if (r.Method != method && r.Method != "*") || !r.regexp().MatchString(path) {
			continue
		}
		if !found || r.wildcards() < best.wildcards() {
			best, found = r, true
		}
	}
	return best, found
}
