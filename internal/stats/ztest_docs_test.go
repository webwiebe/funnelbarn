package stats_test

import (
	"math"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/stats"
)

// TestABTestDocsExampleMatchesZTest reads the analysis response example in the
// A/B test docs and checks its z_score against the product's z-test, so the
// documented number cannot drift from the code.
func TestABTestDocsExampleMatchesZTest(t *testing.T) {
	const docs = "../../site/src/content/docs/ab-tests.md"
	raw, err := os.ReadFile(docs)
	if err != nil {
		t.Fatalf("read %s: %v", docs, err)
	}

	field := func(name string) float64 {
		m := regexp.MustCompile(`"` + name + `":\s*(-?[0-9.]+)`).FindSubmatch(raw)
		if m == nil {
			t.Fatalf("%s: no %q field in the analysis example", docs, name)
		}
		v, err := strconv.ParseFloat(string(m[1]), 64)
		if err != nil {
			t.Fatalf("%s: parse %q: %v", docs, name, err)
		}
		return v
	}

	controlN := int64(field("control_sample"))
	controlX := int64(field("control_conversions"))
	variantN := int64(field("variant_sample"))
	variantX := int64(field("variant_conversions"))
	documented := field("z_score")

	got, significant := stats.ZTestTwoProportions(controlN, controlX, variantN, variantX)
	if math.Abs(got-documented) > 0.01 {
		t.Errorf("docs show z_score %.2f, the z-test gives %.4f for %d/%d vs %d/%d",
			documented, got, controlX, controlN, variantX, variantN)
	}
	if !significant {
		t.Errorf("docs show significant: true, the z-test says not significant (z=%.4f)", got)
	}
}
