package service

import (
	"context"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// FlagRequest is the body of a flag create or update. Every field is a
// pointer so an omitted field differs from an empty one.
//
// On update, an omitted field keeps the stored value, and so does an empty
// string, with one exception: conversion_event "" clears the event, because
// otherwise a caller could never remove it. A status toggle sends only
// {"status": "paused"}.
type FlagRequest struct {
	FlagKey         *string `json:"flag_key"`
	Name            *string `json:"name"`
	FlagType        *string `json:"flag_type"`
	Variants        *string `json:"variants"`
	DefaultVariant  *string `json:"default_variant"`
	Split           *string `json:"split"`
	ConversionEvent *string `json:"conversion_event"`
	TargetingRules  *string `json:"targeting_rules"`
	Status          *string `json:"status"`
	Kind            *string `json:"flag_kind"`
}

// applyTo returns existing with the request's fields laid over it. FlagKey is
// not part of the merge: a flag's key is fixed at creation.
func (r FlagRequest) applyTo(existing repository.FeatureFlag) repository.FeatureFlag {
	merged := existing
	for _, f := range []struct {
		dst *string
		in  *string
	}{
		{&merged.Name, r.Name},
		{&merged.FlagType, r.FlagType},
		{&merged.Variants, r.Variants},
		{&merged.DefaultVariant, r.DefaultVariant},
		{&merged.Split, r.Split},
		{&merged.TargetingRules, r.TargetingRules},
		{&merged.Status, r.Status},
		{&merged.Kind, r.Kind},
	} {
		if f.in != nil && *f.in != "" {
			*f.dst = *f.in
		}
	}
	if r.ConversionEvent != nil {
		merged.ConversionEvent = *r.ConversionEvent
	}
	return merged
}

// CreateFromRequest creates a flag in the project from a request body. The
// same validation as CreateFlag applies.
func (svc *FlagService) CreateFromRequest(ctx context.Context, projectID string, req FlagRequest) (repository.FeatureFlag, error) {
	f := req.applyTo(repository.FeatureFlag{ProjectID: projectID})
	if req.FlagKey != nil {
		f.FlagKey = *req.FlagKey
	}
	return svc.CreateFlag(ctx, f)
}

// PatchFlag lays a partial request over the stored flag and saves it with the
// validation of UpdateFlag.
func (svc *FlagService) PatchFlag(ctx context.Context, existing repository.FeatureFlag, req FlagRequest) (repository.FeatureFlag, error) {
	return svc.UpdateFlag(ctx, req.applyTo(existing))
}
