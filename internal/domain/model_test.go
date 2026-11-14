package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

func TestEnvironmentRule(t *testing.T) {
	require.Nil(t, domain.EnvironmentRule(""))
	require.Nil(t, domain.EnvironmentRule("  "))
	require.Equal(t,
		[]domain.SegmentRule{{Field: "environment", Operator: "eq", Value: "production"}},
		domain.EnvironmentRule("prod"))
	require.Equal(t, "test", domain.EnvironmentRule("QA")[0].Value)
}

// The environment argument must not become a stored-segment field.
func TestEnvironmentIsNotAnAllowedSegmentField(t *testing.T) {
	_, ok := domain.AllowedSegmentFields["environment"]
	require.False(t, ok)
}

func TestValidPropertyKey(t *testing.T) {
	for _, ok := range []string{"plan", "Plan_2", "a_b_C9"} {
		require.True(t, domain.ValidPropertyKey(ok), ok)
	}
	for _, bad := range []string{"", "a.b", "a b", "a'b", "a-b", "$x", strings.Repeat("a", 65)} {
		require.False(t, domain.ValidPropertyKey(bad), bad)
	}
}

func TestValidBreakdownProperty(t *testing.T) {
	require.True(t, domain.ValidBreakdownProperty("surface_1"))
	require.False(t, domain.ValidBreakdownProperty(""))
	require.False(t, domain.ValidBreakdownProperty("a.b"))
}

func TestAPIKeyScopeEvaluatesFlags(t *testing.T) {
	for _, s := range []string{domain.APIKeyScopeIngest, domain.APIKeyScopeFull, domain.APIKeyScopeFlagsRead, domain.APIKeyScopeFlagsWrite} {
		require.True(t, domain.APIKeyScopeEvaluatesFlags(s), s)
	}
	require.False(t, domain.APIKeyScopeEvaluatesFlags(domain.APIKeyScopeAnalyticsRead))
	require.False(t, domain.APIKeyScopeEvaluatesFlags(""))
}

func TestFunnelConversionWindow(t *testing.T) {
	require.Equal(t, domain.DefaultConversionWindow, domain.Funnel{}.ConversionWindow())
	secs := int64(90)
	require.Equal(t, 90*time.Second, domain.Funnel{ConversionWindowSeconds: &secs}.ConversionWindow())
}
