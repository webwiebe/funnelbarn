package environment

import "strings"

const (
	Production  = "production"
	Staging     = "staging"
	Test        = "test"
	Development = "development"
)

var aliases = map[string]string{
	"production":  Production,
	"prod":        Production,
	"prd":         Production,
	"live":        Production,
	"staging":     Staging,
	"stg":         Staging,
	"stage":       Staging,
	"acceptance":  Staging,
	"acc":         Staging,
	"test":        Test,
	"testing":     Test,
	"tst":         Test,
	"qa":          Test,
	"uat":         Test,
	"pr":          Test,
	"e2e":         Test,
	"development": Development,
	"dev":         Development,
	"local":       Development,
	"develop":     Development,
	"workstation": Development,
}

// Normalize maps any alias to its canonical value.
// Unknown or empty input returns Production.
func Normalize(s string) string {
	if s == "" {
		return ""
	}
	if canonical, ok := aliases[strings.ToLower(strings.TrimSpace(s))]; ok {
		return canonical
	}
	return Production
}

// NormalizeEvent maps an ingested event's environment onto a canonical value,
// filing an absent one as production. Deployment config (Normalize) keeps ""
// as "unset" because the API and the fail-closed start-up check both branch on
// it, but an event has no such third state: the analytics filters compare
// environment for equality, so an untagged event is invisible under every
// filter including the "production" one the dashboard defaults to. Tagging it
// production matches where those events actually came from — SDKs that predate
// the environment field only ever ran in production.
//
// A name it does not know is filed as development. A client that names its
// environment and picks something off the list is usually a preview, a CI run or
// a laptop, and filing those as production inflates the numbers people act on. A
// production client with a typo goes missing from production instead, which shows
// on the dashboard at once and is logged by the worker.
func NormalizeEvent(s string) string {
	if strings.TrimSpace(s) == "" {
		return Production
	}
	if !IsKnown(s) {
		return Development
	}
	return Normalize(s)
}

// IsKnown reports whether s is a recognised environment alias. An unrecognised
// value is still filed rather than rejected — dropping an event over a typo is
// worse than mis-filing it — but callers use this to say so out loud.
func IsKnown(s string) bool {
	if s == "" {
		return true
	}
	_, ok := aliases[strings.ToLower(strings.TrimSpace(s))]
	return ok
}

// Canonical returns the four canonical environment values, in reporting order.
func Canonical() []string {
	return []string{Production, Staging, Test, Development}
}
