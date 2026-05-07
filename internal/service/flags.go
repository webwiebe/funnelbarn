package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// FlagEvalResult is the result of evaluating a feature flag.
type FlagEvalResult struct {
	Value   any    `json:"value"`
	Variant string `json:"variant"`
	Reason  string `json:"reason"`
	FlagKey string `json:"flag_key"`
}

type FlagService struct {
	store repository.Querier
}

func NewFlagService(store repository.Querier) *FlagService {
	return &FlagService{store: store}
}

func (svc *FlagService) CreateFlag(ctx context.Context, f repository.FeatureFlag) (repository.FeatureFlag, error) {
	return svc.store.CreateFlag(ctx, f)
}

func (svc *FlagService) GetFlag(ctx context.Context, id string) (repository.FeatureFlag, error) {
	return svc.store.FlagByID(ctx, id)
}

func (svc *FlagService) GetFlagByKey(ctx context.Context, projectID, flagKey string) (repository.FeatureFlag, error) {
	return svc.store.FlagByKey(ctx, projectID, flagKey)
}

func (svc *FlagService) ListFlags(ctx context.Context, projectID string) ([]repository.FeatureFlag, error) {
	return svc.store.ListFlags(ctx, projectID)
}

func (svc *FlagService) UpdateFlag(ctx context.Context, f repository.FeatureFlag) (repository.FeatureFlag, error) {
	return svc.store.UpdateFlag(ctx, f)
}

func (svc *FlagService) DeleteFlag(ctx context.Context, id string) error {
	return svc.store.DeleteFlag(ctx, id)
}

func (svc *FlagService) EvaluateFlag(ctx context.Context, projectID, flagKey string, evalContext map[string]string) (FlagEvalResult, error) {
	ctx, span := tracing.StartSpan(ctx, "flags.evaluate",
		attribute.String("flag.key", flagKey),
		attribute.String("project.id", projectID),
	)
	defer span.End()

	flag, err := svc.store.FlagByKey(ctx, projectID, flagKey)
	if err != nil {
		tracing.RecordError(span, err)
		return FlagEvalResult{}, fmt.Errorf("flag not found: %w", err)
	}

	if flag.Status != "active" {
		span.SetAttributes(attribute.String("flag.reason", "DISABLED"))
		val, _ := variantValue(flag.Variants, flag.DefaultVariant)
		return FlagEvalResult{
			Value:   val,
			Variant: flag.DefaultVariant,
			Reason:  "DISABLED",
			FlagKey: flag.FlagKey,
		}, nil
	}

	targetingKey := evalContext["targetingKey"]
	if targetingKey == "" {
		targetingKey = evalContext["targeting_key"]
	}
	if targetingKey == "" {
		targetingKey = evalContext["session_id"]
	}

	variant := resolveVariant(flag.Split, flag.FlagKey, targetingKey, flag.DefaultVariant)
	val, _ := variantValue(flag.Variants, variant)

	span.SetAttributes(
		attribute.String("flag.variant", variant),
		attribute.String("flag.reason", "SPLIT_EVALUATION"),
	)

	_ = svc.store.RecordEvaluation(ctx, repository.FlagEvaluation{
		FlagID:      flag.ID,
		ProjectID:   flag.ProjectID,
		Variant:     variant,
		ContextHash: hashContext(targetingKey),
		SessionID:   evalContext["session_id"],
	})

	return FlagEvalResult{
		Value:   val,
		Variant: variant,
		Reason:  "SPLIT_EVALUATION",
		FlagKey: flag.FlagKey,
	}, nil
}

func (svc *FlagService) AnalyzeFlag(ctx context.Context, flag repository.FeatureFlag, from, to time.Time) ([]repository.FlagAnalysisResult, error) {
	ctx, span := tracing.StartSpan(ctx, "flags.analyze",
		attribute.String("flag.id", flag.ID),
		attribute.String("flag.key", flag.FlagKey),
	)
	defer span.End()

	evals, err := svc.store.CountEvaluationsByVariant(ctx, flag.ID, from, to)
	if err != nil {
		tracing.RecordError(span, err)
		return nil, fmt.Errorf("count evaluations: %w", err)
	}

	conversions := make(map[string]int64)
	if flag.ConversionEvent != "" {
		conversions, err = svc.store.CountConversionsByVariant(ctx, flag.ID, flag.ConversionEvent, flag.ProjectID, from, to)
		if err != nil {
			return nil, fmt.Errorf("count conversions: %w", err)
		}
	}

	var results []repository.FlagAnalysisResult
	for variant, sample := range evals {
		conv := conversions[variant]
		rate := 0.0
		if sample > 0 {
			rate = float64(conv) / float64(sample)
		}
		results = append(results, repository.FlagAnalysisResult{
			Variant:     variant,
			Sample:      sample,
			Conversions: conv,
			Rate:        rate,
		})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Variant < results[j].Variant })
	return results, nil
}

// resolveVariant deterministically assigns a variant based on split percentages.
func resolveVariant(splitJSON, flagKey, targetingKey, defaultVariant string) string {
	var split map[string]int
	if err := json.Unmarshal([]byte(splitJSON), &split); err != nil || len(split) == 0 {
		return defaultVariant
	}

	h := sha256.Sum256([]byte(targetingKey + ":" + flagKey))
	bucket := binary.BigEndian.Uint64(h[:8]) % 10000

	keys := make([]string, 0, len(split))
	for k := range split {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var cumulative uint64
	for _, k := range keys {
		cumulative += uint64(split[k]) * 100 // percent → basis points
		if bucket < cumulative {
			return k
		}
	}
	return defaultVariant
}

func variantValue(variantsJSON, variant string) (any, error) {
	var variants map[string]any
	if err := json.Unmarshal([]byte(variantsJSON), &variants); err != nil {
		return nil, err
	}
	v, ok := variants[variant]
	if !ok {
		return nil, fmt.Errorf("variant %q not found", variant)
	}
	return v, nil
}

func hashContext(targetingKey string) string {
	h := sha256.Sum256([]byte(targetingKey))
	return fmt.Sprintf("%x", h[:16])
}
