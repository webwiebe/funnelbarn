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
