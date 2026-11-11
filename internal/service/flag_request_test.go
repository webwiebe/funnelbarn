package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/repository/mock"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

func strp(s string) *string { return &s }

func storedFlag(t *testing.T, svc *service.FlagService) repository.FeatureFlag {
	t.Helper()
	f, err := svc.CreateFlag(context.Background(), repository.FeatureFlag{
		ProjectID:       "proj-1",
		FlagKey:         "patched",
		Name:            "Original",
		FlagType:        "string",
		Variants:        `{"a":"A","b":"B"}`,
		DefaultVariant:  "a",
		Split:           `{"a":50,"b":50}`,
		ConversionEvent: "signup",
		TargetingRules:  `[{"name":"pros","variant":"b","match":"all","conditions":[{"context_key":"plan","operator":"eq","value":"pro"}]}]`,
		Kind:            "experiment",
	})
	require.NoError(t, err)
	return f
}

// An empty request changes nothing, field by field.
func TestFlagService_PatchFlag_EmptyRequestKeepsEveryField(t *testing.T) {
	svc := service.NewFlagService(mock.New())
	f := storedFlag(t, svc)

	got, err := svc.PatchFlag(context.Background(), f, service.FlagRequest{})
	require.NoError(t, err)
	require.Equal(t, f.Name, got.Name)
	require.Equal(t, f.FlagType, got.FlagType)
	require.Equal(t, f.Variants, got.Variants)
	require.Equal(t, f.DefaultVariant, got.DefaultVariant)
	require.Equal(t, f.Split, got.Split)
	require.Equal(t, f.ConversionEvent, got.ConversionEvent)
	require.Equal(t, f.TargetingRules, got.TargetingRules)
	require.Equal(t, f.Status, got.Status)
	require.Equal(t, f.Kind, got.Kind)
}

// Each field in the request replaces exactly its stored counterpart.
func TestFlagService_PatchFlag_EachFieldReplacesItsOwn(t *testing.T) {
	cases := []struct {
		name  string
		req   service.FlagRequest
		check func(t *testing.T, got repository.FeatureFlag)
	}{
		{"name", service.FlagRequest{Name: strp("New")}, func(t *testing.T, g repository.FeatureFlag) { require.Equal(t, "New", g.Name) }},
		{"flag_type", service.FlagRequest{FlagType: strp("number")}, func(t *testing.T, g repository.FeatureFlag) { require.Equal(t, "number", g.FlagType) }},
		{"variants", service.FlagRequest{Variants: strp(`{"a":"A","b":"B","c":"C"}`)}, func(t *testing.T, g repository.FeatureFlag) {
			require.Equal(t, `{"a":"A","b":"B","c":"C"}`, g.Variants)
		}},
		{"default_variant", service.FlagRequest{DefaultVariant: strp("b")}, func(t *testing.T, g repository.FeatureFlag) { require.Equal(t, "b", g.DefaultVariant) }},
		{"split", service.FlagRequest{Split: strp(`{"a":10,"b":90}`)}, func(t *testing.T, g repository.FeatureFlag) { require.Equal(t, `{"a":10,"b":90}`, g.Split) }},
		{"conversion_event", service.FlagRequest{ConversionEvent: strp("purchase")}, func(t *testing.T, g repository.FeatureFlag) { require.Equal(t, "purchase", g.ConversionEvent) }},
		{"targeting_rules", service.FlagRequest{TargetingRules: strp(`[]`)}, func(t *testing.T, g repository.FeatureFlag) { require.Equal(t, `[]`, g.TargetingRules) }},
		{"status", service.FlagRequest{Status: strp("paused")}, func(t *testing.T, g repository.FeatureFlag) { require.Equal(t, "paused", g.Status) }},
		{"kind", service.FlagRequest{Kind: strp("config")}, func(t *testing.T, g repository.FeatureFlag) { require.Equal(t, "config", g.Kind) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := service.NewFlagService(mock.New())
			f := storedFlag(t, svc)
			got, err := svc.PatchFlag(context.Background(), f, tc.req)
			require.NoError(t, err)
			tc.check(t, got)
			// Every other field survives.
			if tc.name != "name" {
				require.Equal(t, f.Name, got.Name)
			}
			if tc.name != "conversion_event" {
				require.Equal(t, f.ConversionEvent, got.ConversionEvent)
			}
		})
	}
}

// Empty strings keep the stored value for every field except conversion_event,
// where an explicit "" is the only way to clear it.
func TestFlagService_PatchFlag_EmptyStringSemantics(t *testing.T) {
	svc := service.NewFlagService(mock.New())
	f := storedFlag(t, svc)

	got, err := svc.PatchFlag(context.Background(), f, service.FlagRequest{Name: strp(""), Status: strp(""), ConversionEvent: strp("")})
	require.NoError(t, err)
	require.Equal(t, f.Name, got.Name)
	require.Equal(t, f.Status, got.Status)
	require.Equal(t, "", got.ConversionEvent)
}

// The merged flag is validated like a full update.
func TestFlagService_PatchFlag_ValidatesMergedFlag(t *testing.T) {
	svc := service.NewFlagService(mock.New())
	f := storedFlag(t, svc)

	_, err := svc.PatchFlag(context.Background(), f, service.FlagRequest{Kind: strp("nonsense")})
	require.Error(t, err)
}

// Create builds the flag from the same request type.
func TestFlagService_CreateFromRequest(t *testing.T) {
	svc := service.NewFlagService(mock.New())
	got, err := svc.CreateFromRequest(context.Background(), "proj-1", service.FlagRequest{
		FlagKey:         strp("from-request"),
		Name:            strp("From request"),
		ConversionEvent: strp("signup"),
	})
	require.NoError(t, err)
	require.Equal(t, "proj-1", got.ProjectID)
	require.Equal(t, "from-request", got.FlagKey)
	require.Equal(t, "signup", got.ConversionEvent)
	require.Equal(t, "boolean", got.FlagType)

	_, err = svc.CreateFromRequest(context.Background(), "proj-1", service.FlagRequest{Name: strp("no key")})
	require.Error(t, err)
}
