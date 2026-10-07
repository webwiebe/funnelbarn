package api

import (
	"strings"
	"testing"
)

// TestGoldenRouteCoverage derives every route (all methods) from the package
// source and fails for any that has neither a golden case nor an entry in
// goldenSkips. A route registered without a method counts as method "*".
func TestGoldenRouteCoverage(t *testing.T) {
	routes := goldenParseRoutes(t)
	covered := map[string]bool{}
	for _, c := range goldenAllCases(goldenPlaceholderSeed()) {
		path := strings.SplitN(c.Path, "?", 2)[0]
		r, ok := goldenMatchRoute(routes, c.Method, path)
		if !ok {
			t.Errorf("case %s: %s %s matches no registered route", c.Name, c.Method, path)
			continue
		}
		covered[r.String()] = true
	}

	known := map[string]bool{}
	for _, r := range routes {
		known[r.String()] = true
		_, skipped := goldenSkips[r.String()]
		switch {
		case covered[r.String()] && skipped:
			t.Errorf("%s has a golden case and a skip entry; remove the skip", r)
		case !covered[r.String()] && !skipped:
			t.Errorf("route %s has no golden case: add one in golden_cases*_test.go, or add a justified entry to goldenSkips", r)
		}
	}
	for route := range goldenSkips {
		if !known[route] {
			t.Errorf("goldenSkips entry %q matches no registered route; remove it", route)
		}
	}
	t.Logf("%d routes parsed, %d skipped", len(routes), len(goldenSkips))
}
