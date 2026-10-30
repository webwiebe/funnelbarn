package mcp

import (
	"context"
	"encoding/json"

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
	TargetingRules string `json:"targeting_rules,omitempty" jsonschema:"JSON array of targeting rules; defaults to []. Each rule needs name, variant (\"default\" or a name in variants), match (\"all\"/\"any\") and conditions (context_key, operator, value)."`
	// Variants is a map so an assistant can write {"default": false, "on": true}
	// directly. The flag's other variant fields stay raw JSON strings.
	Variants map[string]any `json:"variants,omitempty" jsonschema:"Variant name to value, e.g. {\"default\":false,\"on\":true}, for targeting rules to return. The flag's default variant is always \"default\", set here or through default_value."`
}

// createVariants builds a new flag's variants. "default" is always among them,
// since it is the flag's default_variant.
func createVariants(in createFlagIn) (map[string]any, error) {
	if _, set := in.Variants["default"]; set && in.DefaultValue != nil {
		return nil, invalidInput("set either default_value or variants.default")
	}
	variants := map[string]any{"default": false}
	if in.DefaultValue != nil {
		variants["default"] = in.DefaultValue
	}
	for name, value := range in.Variants {
		if name == "" {
			return nil, invalidInput("variants: a variant needs a name")
		}
		if value == nil {
			return nil, invalidInput("variants: %q has no value", name)
		}
		variants[name] = value
	}
	return variants, nil
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
	variants, err := createVariants(in)
	if err != nil {
		return flagDetail{}, err
	}
	variantsJSON, err := json.Marshal(variants)
	if err != nil {
		return flagDetail{}, invalidInput("variants: %v", err)
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
		FlagType:       service.InferFlagType(variants["default"]),
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
	TargetingRules string `json:"targeting_rules,omitempty" jsonschema:"JSON array of targeting rules; omit to keep the current rules. Every rule's variant must exist on the flag."`
	Status         string `json:"status,omitempty" jsonschema:"\"active\" or \"paused\"; omit to keep the current status."`
	// Variants is a map for the same reason as on createFlagIn.
	Variants map[string]any `json:"variants,omitempty" jsonschema:"Variants to change, merged by name: a value adds or sets that variant and null removes it. Variants you omit stay. A variant in use as the default, in a rule or in the split cannot be removed."`
}

// updatedVariants applies an update's default_value and variants to the flag's
// stored variants JSON, leaving any other variant untouched, so an update on a
// flag with more than one variant (created from the dashboard) doesn't clobber
// the others.
func updatedVariants(existing repository.FeatureFlag, in updateFlagIn) (string, error) {
	if in.DefaultValue == nil && len(in.Variants) == 0 {
		return existing.Variants, nil
	}
	variants := map[string]any{}
	if existing.Variants != "" {
		if err := json.Unmarshal([]byte(existing.Variants), &variants); err != nil {
			return "", invalidInput("existing variants: %v", err)
		}
	}
	key := existing.DefaultVariant
	if key == "" {
		key = "default"
	}
	if _, set := in.Variants[key]; set && in.DefaultValue != nil {
		return "", invalidInput("set either default_value or variants.%s", key)
	}
	if in.DefaultValue != nil {
		variants[key] = in.DefaultValue
	}
	for name, value := range in.Variants {
		if err := applyVariant(variants, existing, key, name, value); err != nil {
			return "", err
		}
	}
	b, err := json.Marshal(variants)
	if err != nil {
		return "", invalidInput("variants: %v", err)
	}
	return string(b), nil
}

// applyVariant sets one variant, or removes it when value is nil. FlagService
// rejects the removal of a variant a rule names, when it validates the rules.
func applyVariant(variants map[string]any, existing repository.FeatureFlag, defaultVariant, name string, value any) error {
	if name == "" {
		return invalidInput("variants: a variant needs a name")
	}
	if value != nil {
		variants[name] = value
		return nil
	}
	if name == defaultVariant {
		return invalidInput("variants: %q is the default variant and cannot be removed", name)
	}
	if _, ok := variants[name]; !ok {
		return invalidInput("variants: %q does not exist", name)
	}
	split := map[string]any{}
	if err := json.Unmarshal([]byte(existing.Split), &split); err == nil {
		if _, used := split[name]; used {
			return invalidInput("variants: %q is in the split and cannot be removed", name)
		}
	}
	delete(variants, name)
	return nil
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
	variants, err := updatedVariants(existing, in)
	if err != nil {
		return flagDetail{}, err
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
