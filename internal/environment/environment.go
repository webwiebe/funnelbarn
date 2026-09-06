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
	"development": Development,
	"dev":         Development,
	"local":       Development,
	"develop":     Development,
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
func NormalizeEvent(s string) string {
	if canonical := Normalize(s); canonical != "" {
		return canonical
	}
	return Production
}

// IsKnown reports whether s is a recognised environment alias. An unrecognised
// value still normalises to Production rather than being rejected — dropping an
// event over a typo is worse than mis-filing it — but callers use this to say
// so out loud instead of silently mis-filing "prodution" as production.
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
