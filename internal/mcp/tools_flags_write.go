package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

// ---------------------------------------------------------------------------
// create_flag
// ---------------------------------------------------------------------------

type createFlagIn struct {
	Project string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	FlagKey string `json:"flag_key" jsonschema:"Unique key SDKs will evaluate by, e.g. \"new_checkout\". Required."`
	// Description names the argument the way an assistant thinks of it (what
	// is this flag for); it maps onto the flag's Name, the only human-readable
	// label repository.FeatureFlag has.
	Description    string `json:"description,omitempty" jsonschema:"Human-readable name shown on the Flags page; defaults to flag_key."`
	Kind           string `json:"flag_kind,omitempty" jsonschema:"\"experiment\" (default; bucketed per user, every evaluation recorded) or \"config\" (one value read on a loop)."`
	DefaultValue   any    `json:"default_value,omitempty" jsonschema:"Value returned when no targeting rule matches. Defaults to false."`
	TargetingRules string `json:"targeting_rules,omitempty" jsonschema:"JSON array of targeting rules; defaults to []. Each rule needs name, variant (must be \"default\" unless you update the flag with more variants), match (\"all\"/\"any\") and conditions (context_key, operator, value)."`
}

func createFlag(ctx context.Context, c *Call, in createFlagIn) (flagDetail, error) {
	if in.FlagKey == "" {
		return flagDetail{}, invalidInput("flag_key is required")
	}
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return flagDetail{}, err
	}
	name := in.Description
	if name == "" {
		name = in.FlagKey
	}
	defaultValue := in.DefaultValue
	if defaultValue == nil {
		defaultValue = false
	}
	variantsJSON, err := json.Marshal(map[string]any{"default": defaultValue})
	if err != nil {
		return flagDetail{}, invalidInput("default_value: %v", err)
	}
	targetingRules := in.TargetingRules
	if targetingRules == "" {
		targetingRules = "[]"
	}

	// FlagService.CreateFlag validates flag_key/name, defaults flag_type,
	// validates targeting_rules and normalizes flag_kind, and always sets
	// origin "manual". handleCreateFlag uses the same path.
	f, err := c.Deps.Flags.CreateFlag(ctx, repository.FeatureFlag{
		ProjectID:      p.ID,
		FlagKey:        in.FlagKey,
		Name:           name,
		FlagType:       service.InferFlagType(defaultValue),
		Variants:       string(variantsJSON),
		DefaultVariant: "default",
		Split:          "{}",
		TargetingRules: targetingRules,
		Kind:           in.Kind,
	})
	if err != nil {
		return flagDetail{}, err
	}
	return toFlagDetail(f), nil
}

// ---------------------------------------------------------------------------
// update_flag
// ---------------------------------------------------------------------------

type updateFlagIn struct {
	Project        string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	FlagKey        string `json:"flag_key,omitempty" jsonschema:"The flag's key. Either flag_key or flag_id is required."`
	FlagID         string `json:"flag_id,omitempty" jsonschema:"The flag's ID. Either flag_key or flag_id is required."`
	Name           string `json:"name,omitempty" jsonschema:"New name; omit to keep the current name."`
	DefaultValue   any    `json:"default_value,omitempty" jsonschema:"New value for the flag's default_variant; omit to keep the current value. Cannot be set to null."`
	Kind           string `json:"flag_kind,omitempty" jsonschema:"\"experiment\" or \"config\"; omit to keep the current kind."`
	TargetingRules string `json:"targeting_rules,omitempty" jsonschema:"JSON array of targeting rules; omit to keep the current rules."`
	Status         string `json:"status,omitempty" jsonschema:"\"active\" or \"paused\"; omit to keep the current status."`
}

// mergeFlagVariantValue sets value under the flag's own default_variant key
// inside its existing variants JSON, leaving any other variant untouched, so
// updating default_value on a flag with more than one variant (created from
// the dashboard) doesn't clobber the others.
func mergeFlagVariantValue(existing repository.FeatureFlag, value any) (string, error) {
	variants := map[string]any{}
	if existing.Variants != "" {
		if err := json.Unmarshal([]byte(existing.Variants), &variants); err != nil {
			return "", fmt.Errorf("existing variants: %w", err)
		}
	}
	key := existing.DefaultVariant
	if key == "" {
		key = "default"
	}
	variants[key] = value
	b, err := json.Marshal(variants)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func updateFlag(ctx context.Context, c *Call, in updateFlagIn) (flagDetail, error) {
	_, existing, err := resolveFlag(ctx, c, in.Project, in.FlagKey, in.FlagID)
	if err != nil {
		return flagDetail{}, err
	}

	name := existing.Name
	if in.Name != "" {
		name = in.Name
	}
	variants := existing.Variants
	if in.DefaultValue != nil {
		variants, err = mergeFlagVariantValue(existing, in.DefaultValue)
		if err != nil {
			return flagDetail{}, invalidInput("default_value: %v", err)
		}
	}
	kind := existing.Kind
	if in.Kind != "" {
		kind = in.Kind
	}
	targetingRules := existing.TargetingRules
	if in.TargetingRules != "" {
		targetingRules = in.TargetingRules
	}
	status := existing.Status
	if in.Status != "" {
		status = in.Status
	}

	// FlagService.UpdateFlag validates targeting_rules and normalizes
	// flag_kind, and the store claims the flag as origin "manual" on any
	// update. handleUpdateFlag uses the same path.
	updated, err := c.Deps.Flags.UpdateFlag(ctx, repository.FeatureFlag{
		ID:              existing.ID,
		Name:            name,
		FlagType:        existing.FlagType,
		Variants:        variants,
		DefaultVariant:  existing.DefaultVariant,
		Split:           existing.Split,
		ConversionEvent: existing.ConversionEvent,
		TargetingRules:  targetingRules,
		Status:          status,
		Kind:            kind,
	})
	if err != nil {
		return flagDetail{}, err
	}
	return toFlagDetail(updated), nil
}

// ---------------------------------------------------------------------------
// delete_flag
// ---------------------------------------------------------------------------

type deleteFlagIn struct {
	Project string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	FlagKey string `json:"flag_key,omitempty" jsonschema:"The flag's key. Either flag_key or flag_id is required."`
	FlagID  string `json:"flag_id,omitempty" jsonschema:"The flag's ID. Either flag_key or flag_id is required."`
}

type deleteFlagOut struct {
	Deleted bool   `json:"deleted" jsonschema:"Always true on success."`
	FlagKey string `json:"flag_key" jsonschema:"The deleted flag's key."`
}

func deleteFlag(ctx context.Context, c *Call, in deleteFlagIn) (deleteFlagOut, error) {
	_, f, err := resolveFlag(ctx, c, in.Project, in.FlagKey, in.FlagID)
	if err != nil {
		return deleteFlagOut{}, err
	}
	if err := c.Deps.Flags.DeleteFlag(ctx, f.ID); err != nil {
		return deleteFlagOut{}, err
	}
	return deleteFlagOut{Deleted: true, FlagKey: f.FlagKey}, nil
}
