package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/bblog"
	"github.com/wiebe-xyz/funnelbarn/internal/domain"
	"github.com/wiebe-xyz/funnelbarn/internal/ports"
	"github.com/wiebe-xyz/funnelbarn/internal/repository"
	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// FlagEvalResult follows the OpenFeature resolution details structure.
type FlagEvalResult struct {
	Value        any            `json:"value"`
	Variant      string         `json:"variant"`
	Reason       string         `json:"reason"`
	FlagKey      string         `json:"flag_key"`
	ErrorCode    string         `json:"error_code,omitempty"`
	FlagMetadata map[string]any `json:"flag_metadata,omitempty"`
	// CacheMaxAgeSeconds is how long the caller may reuse this result without
	// re-evaluating. It is the one sanctioned polling interval, so every
	// consumer doesn't invent its own. Zero means "do not cache": an experiment
	// resolves per targeting key and each read is a data point, so caching one
	// would both mis-bucket and silently drop the analytics.
	CacheMaxAgeSeconds int `json:"cache_max_age_seconds"`
}

// DefaultConfigCacheTTL is how long a config flag's value may be cached by
// default. Long enough that a fleet polling a value costs the database almost
// nothing; short enough that widening a cap from the dashboard takes effect
// while you are still looking at the page.
const DefaultConfigCacheTTL = 60 * time.Second

type FlagService struct {
	store          ports.FlagRepo
	configCacheTTL time.Duration

	// touchedMu guards the last_evaluated_at write throttle.
	touchedMu sync.Mutex
	touchedAt map[string]time.Time
}

func NewFlagService(store ports.FlagRepo) *FlagService {
	return &FlagService{
		store:          store,
		configCacheTTL: DefaultConfigCacheTTL,
		touchedAt:      make(map[string]time.Time),
	}
}

// WithConfigCacheTTL overrides the cache hint returned for config flags.
// A non-positive ttl restores the default.
func (svc *FlagService) WithConfigCacheTTL(ttl time.Duration) *FlagService {
	if ttl <= 0 {
		ttl = DefaultConfigCacheTTL
	}
	svc.configCacheTTL = ttl
	return svc
}

// cacheHintSeconds is the max-age advertised for a resolved flag.
func (svc *FlagService) cacheHintSeconds(flag repository.FeatureFlag) int {
	if flag.Kind != repository.FlagKindConfig {
		return 0
	}
	ttl := svc.configCacheTTL
	if ttl <= 0 {
		ttl = DefaultConfigCacheTTL
	}
	return int(ttl.Seconds())
}

// NormalizeFlagKind defaults an unset kind to "experiment" (what every flag
// was before the column existed) and rejects anything else, so a typo can't
// create a flag whose evaluation semantics nobody can predict.
func NormalizeFlagKind(kind string) (string, error) {
	switch kind {
	case "":
		return repository.FlagKindExperiment, nil
	case repository.FlagKindExperiment, repository.FlagKindConfig:
		return kind, nil
	default:
		return "", fmt.Errorf("flag_kind must be %q or %q", repository.FlagKindExperiment, repository.FlagKindConfig)
	}
}

// CreateFlag validates and creates a manually-authored flag: flag_key and
// name are required, flag_type defaults to "boolean", targeting_rules to
// "[]", and the kind is normalized (see NormalizeFlagKind). Origin is always
// "manual": this is the dashboard, API and MCP creation path. Auto-registered flags
// go through EnsureAutoFlag (via EvaluateOrRegisterFlag) instead and never
// reach here, so they keep working unchanged.
func (svc *FlagService) CreateFlag(ctx context.Context, f repository.FeatureFlag) (repository.FeatureFlag, error) {
	if f.FlagKey == "" {
		return repository.FeatureFlag{}, &domain.ValidationError{Field: "flag_key", Message: "is required"}
	}
	if f.Name == "" {
		return repository.FeatureFlag{}, &domain.ValidationError{Field: "name", Message: "is required"}
	}
	if f.FlagType == "" {
		f.FlagType = "boolean"
	}
	if f.TargetingRules == "" {
		f.TargetingRules = "[]"
	}
	if err := ValidateTargetingRules(f.TargetingRules); err != nil {
		return repository.FeatureFlag{}, &domain.ValidationError{Message: err.Error()}
	}
	kind, err := NormalizeFlagKind(f.Kind)
	if err != nil {
		return repository.FeatureFlag{}, &domain.ValidationError{Field: "flag_kind", Message: err.Error()}
	}
	f.Kind = kind
	if f.Status == "" {
		f.Status = "active"
	}
	f.Origin = "manual"
	return svc.store.CreateFlag(ctx, f)
}

func (svc *FlagService) GetFlag(ctx context.Context, id string) (repository.FeatureFlag, error) {
	f, err := svc.store.FlagByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.FeatureFlag{}, fmt.Errorf("%w: flag %s", domain.ErrNotFound, id)
		}
		return repository.FeatureFlag{}, err
	}
	return f, nil
}

func (svc *FlagService) GetFlagByKey(ctx context.Context, projectID, flagKey string) (repository.FeatureFlag, error) {
	f, err := svc.store.FlagByKey(ctx, projectID, flagKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return repository.FeatureFlag{}, fmt.Errorf("%w: flag %s", domain.ErrNotFound, flagKey)
		}
		return repository.FeatureFlag{}, err
	}
	return f, nil
}

func (svc *FlagService) ListFlags(ctx context.Context, projectID string) ([]repository.FeatureFlag, error) {
	return svc.store.ListFlags(ctx, projectID)
}

// UpdateFlag validates and saves an update to an existing flag: targeting
// rules must be well-formed and the kind is normalized (see
// NormalizeFlagKind). The caller merges a partial update (see internal/api's
// handleUpdateFlag), because only the caller knows which fields the request
// actually included.
func (svc *FlagService) UpdateFlag(ctx context.Context, f repository.FeatureFlag) (repository.FeatureFlag, error) {
	if f.Name == "" {
		return repository.FeatureFlag{}, &domain.ValidationError{Field: "name", Message: "is required"}
	}
	if f.TargetingRules == "" {
		f.TargetingRules = "[]"
	}
	if err := ValidateTargetingRules(f.TargetingRules); err != nil {
		return repository.FeatureFlag{}, &domain.ValidationError{Message: err.Error()}
	}
	kind, err := NormalizeFlagKind(f.Kind)
	if err != nil {
		return repository.FeatureFlag{}, &domain.ValidationError{Field: "flag_kind", Message: err.Error()}
	}
	f.Kind = kind
	return svc.store.UpdateFlag(ctx, f)
}

func (svc *FlagService) DeleteFlag(ctx context.Context, id string) error {
	return svc.store.DeleteFlag(ctx, id)
}

func (svc *FlagService) EvaluateFlag(ctx context.Context, projectID, flagKey string, evalContext map[string]any) (FlagEvalResult, error) {
	return svc.evaluateFlag(ctx, projectID, flagKey, evalContext, true)
}

// PreviewFlag evaluates a flag exactly like EvaluateFlag but writes no
// evaluation row, so a dry run from an assistant never shows up in the flag's
// exposure counts or A/B analysis.
func (svc *FlagService) PreviewFlag(ctx context.Context, projectID, flagKey string, evalContext map[string]any) (FlagEvalResult, error) {
	return svc.evaluateFlag(ctx, projectID, flagKey, evalContext, false)
}

func (svc *FlagService) evaluateFlag(ctx context.Context, projectID, flagKey string, evalContext map[string]any, record bool) (FlagEvalResult, error) {
	ctx, span := tracing.StartSpan(ctx, "flags.evaluate",
		attribute.String("flag.key", flagKey),
		attribute.String("project.id", projectID),
	)
	defer span.End()

	flag, err := svc.store.FlagByKey(ctx, projectID, flagKey)
	if err != nil {
		// A missing flag is an expected, well-defined outcome — the caller gets
		// its own default (FLAG_NOT_FOUND), and auto-registration is built on
		// exactly this path. Recording it as a span error made every evaluation
		// of an unregistered flag look like a fault in SpanBarn.
		if errors.Is(err, sql.ErrNoRows) {
			span.SetAttributes(attribute.String("flag.reason", "FLAG_NOT_FOUND"))
		} else {
			tracing.RecordError(span, err)
		}
		return FlagEvalResult{}, fmt.Errorf("flag not found: %w", err)
	}

	if flag.Status != "active" {
		span.SetAttributes(attribute.String("flag.reason", "DISABLED"))
		val, _ := variantValue(flag.Variants, flag.DefaultVariant)
		return FlagEvalResult{
			Value:              val,
			Variant:            flag.DefaultVariant,
			Reason:             "DISABLED",
			FlagKey:            flag.FlagKey,
			CacheMaxAgeSeconds: svc.cacheHintSeconds(flag),
		}, nil
	}

	// A config flag is a singleton value polled by a server: no user, no bucket,
	// no conversion. Recording a row per read would write thousands a day for a
	// value that changes twice, and would drown the flag's own analytics in
	// machine reads.
	static := flag.Kind == repository.FlagKindConfig
	records := record && !static
	span.SetAttributes(attribute.String("flag.kind", flag.Kind), attribute.Bool("flag.preview", !record))

	targetingKey := contextString(evalContext, "targetingKey")
	if targetingKey == "" {
		targetingKey = contextString(evalContext, "targeting_key")
	}
	if targetingKey == "" {
		targetingKey = contextString(evalContext, "session_id")
	}
	sessionID := contextString(evalContext, "session_id")

	ctxKeys := contextKeyNames(evalContext)

	if variant, ruleName, matched := evaluateTargetingRules(flag.TargetingRules, evalContext); matched {
		val, _ := variantValue(flag.Variants, variant)
		span.SetAttributes(
			attribute.String("flag.variant", variant),
			attribute.String("flag.reason", "TARGETING_MATCH"),
			attribute.String("flag.rule_name", ruleName),
		)
		if records {
			svc.recordEvaluation(ctx, flag, variant, targetingKey, sessionID, ctxKeys, "targeting")
		}
		return FlagEvalResult{
			Value:              val,
			Variant:            variant,
			Reason:             "TARGETING_MATCH",
			FlagKey:            flag.FlagKey,
			FlagMetadata:       map[string]any{"evaluated_rule_name": ruleName},
			CacheMaxAgeSeconds: svc.cacheHintSeconds(flag),
		}, nil
	}

	// A config flag holds one value for everyone. Bucketing it by targeting key
	// would let separate pods read different values for the same setting, which
	// is exactly the failure the kind exists to prevent.
	if static {
		val, _ := variantValue(flag.Variants, flag.DefaultVariant)
		span.SetAttributes(
			attribute.String("flag.variant", flag.DefaultVariant),
			attribute.String("flag.reason", "STATIC"),
		)
		return FlagEvalResult{
			Value:              val,
			Variant:            flag.DefaultVariant,
			Reason:             "STATIC",
			FlagKey:            flag.FlagKey,
			CacheMaxAgeSeconds: svc.cacheHintSeconds(flag),
		}, nil
	}

	variant := resolveVariant(flag.Split, flag.FlagKey, targetingKey, flag.DefaultVariant)
	val, _ := variantValue(flag.Variants, variant)

	span.SetAttributes(
		attribute.String("flag.variant", variant),
		attribute.String("flag.reason", "SPLIT"),
	)

	if records {
		svc.recordEvaluation(ctx, flag, variant, targetingKey, sessionID, ctxKeys, "split")
	}

	return FlagEvalResult{
		Value:   val,
		Variant: variant,
		Reason:  "SPLIT",
		FlagKey: flag.FlagKey,
	}, nil
}

// recordEvaluation writes the analytics row for one evaluation. It is
// best-effort: the evaluation already happened and only the row is lost, so a
// storage failure is logged at Warn and the caller still gets its value.
func (svc *FlagService) recordEvaluation(ctx context.Context, flag repository.FeatureFlag, variant, targetingKey, sessionID string, ctxKeys []string, path string) {
	if err := svc.store.RecordEvaluation(ctx, repository.FlagEvaluation{
		FlagID:      flag.ID,
		ProjectID:   flag.ProjectID,
		Variant:     variant,
		ContextHash: hashContext(targetingKey),
		SessionID:   sessionID,
		ContextKeys: ctxKeys,
	}); err != nil {
		slog.WarnContext(ctx, "flag: record evaluation ("+path+")",
			"err", err, "handled", true,
			"flag_id", flag.ID, "project_id", flag.ProjectID)
	}
}

// flagKeyRe bounds auto-registration to sane keys so a spam caller can't create
// rows with arbitrary/oversized garbage keys.
var flagKeyRe = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

func validFlagKey(key string) bool { return flagKeyRe.MatchString(key) }

// InferFlagType maps a JSON-decoded default value to a flag_type for the
// dashboard. Exported so callers outside the service (e.g. the MCP tools'
// create_flag) can build a repository.FeatureFlag from a bare default value
// the same way EvaluateOrRegisterFlag's auto-registration does.
func InferFlagType(v any) string {
	switch v.(type) {
	case bool:
		return "boolean"
	case float64, int, int64:
		return "number"
	case string, nil:
		return "string"
	default:
		return "json"
	}
}

// buildAutoFlag describes an inert, auto-created flag: a single "default" variant
// holding the caller's default value, status "inactive" so evaluation returns that
// default (reason DISABLED) until a human configures it.
func buildAutoFlag(projectID, flagKey string, defaultValue any, kind string) repository.FeatureFlag {
	if kind != repository.FlagKindConfig {
		kind = repository.FlagKindExperiment
	}
	variants := map[string]any{"default": defaultValue}
	vb, err := json.Marshal(variants)
	if err != nil {
		vb = []byte(`{"default":null}`)
	}
	return repository.FeatureFlag{
		ProjectID:      projectID,
		FlagKey:        flagKey,
		Name:           flagKey,
		FlagType:       InferFlagType(defaultValue),
		Variants:       string(vb),
		DefaultVariant: "default",
		Split:          "{}",
		TargetingRules: "[]",
		Status:         "inactive",
		Origin:         "auto",
		Kind:           kind,
	}
}

// EvaluateOrRegisterFlag evaluates a flag and, when it doesn't exist yet,
// auto-registers an inert flag carrying the caller's default so it surfaces in
// the dashboard ready to configure. The caller always gets its default back in
// that case (reason DISABLED), so SDK behaviour is unchanged.
//
// maxAuto caps auto-created flags per project (0 disables auto-registration).
// Invalid keys, a missing project, or hitting the cap fall back to the original
// not-found behaviour (the cap case via domain.ErrAutoRegisterLimit).
// kind is the caller's declared flag kind, honoured only when the flag is
// auto-created here; an existing flag keeps whatever the dashboard says.
func (svc *FlagService) EvaluateOrRegisterFlag(ctx context.Context, projectID, flagKey string, evalContext map[string]any, defaultValue any, maxAuto int, kind string) (FlagEvalResult, error) {
	res, err := svc.EvaluateFlag(ctx, projectID, flagKey, evalContext)
	if err == nil {
		// Record that the flag was evaluated. This used to fire only for
		// origin='auto' flags returning DISABLED, which is the intersection of
		// two gates that between them excluded every flag actually in service:
		// a live manual flag returns a real reason, not DISABLED, and would be
		// filtered on origin anyway. last_evaluated_at therefore marked the
		// flags nobody used and left the busy ones reading as never-evaluated.
		svc.touchEvaluated(projectID, flagKey)
		return res, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return FlagEvalResult{}, err // a real lookup failure, not a missing flag
	}
	if maxAuto <= 0 || projectID == "" || !validFlagKey(flagKey) {
		return FlagEvalResult{}, err // fall through to FLAG_NOT_FOUND + default
	}
	if n, cerr := svc.store.CountAutoFlags(ctx, projectID); cerr == nil && n >= maxAuto {
		return FlagEvalResult{}, fmt.Errorf("project %s: %w", projectID, domain.ErrAutoRegisterLimit)
	}
	if _, cerr := svc.store.EnsureAutoFlag(ctx, buildAutoFlag(projectID, flagKey, defaultValue, kind)); cerr != nil {
		slog.WarnContext(ctx, "flag: auto-register failed", "err", cerr, "handled", true,
			"project_id", projectID, "flag_key", flagKey)
		// Still hand the caller its default so the SDK is unaffected.
		return FlagEvalResult{Value: defaultValue, Variant: "default", Reason: "DISABLED", FlagKey: flagKey}, nil
	}
	// Re-evaluate now that the inert flag exists (returns default, reason
	// DISABLED). Record this evaluation too: the flag is brand new, so the
	// sweep's COALESCE would fall back to created_at either way, but leaving it
	// null here means a flag auto-registered and then evaluated once a day
	// keeps reading as never-evaluated.
	res, err = svc.EvaluateFlag(ctx, projectID, flagKey, evalContext)
	if err == nil {
		svc.touchEvaluated(projectID, flagKey)
	}
	return res, err
}

// touchInterval throttles the last_evaluated_at write to at most one per flag
// per minute. The gate it replaces was an origin filter, which limited write
// amplification by excluding the flags that generate the most evaluations —
// exactly the ones the column needs to be right about. A time throttle applies
// the same concern uniformly: staleness is measured in days, so a minute's
// resolution costs nothing and a flag served a thousand times a second still
// produces one write per minute.
const touchInterval = time.Minute

// maxTrackedTouches bounds the throttle map. Flags per instance are few (tens),
// but auto-registration can mint them, so the map is dropped rather than grown
// without limit; the only cost of losing it is one extra write per flag.
const maxTrackedTouches = 4096

// shouldTouch reports whether this flag's last_evaluated_at is due for a write,
// recording the decision so the next evaluation within touchInterval skips it.
func (svc *FlagService) shouldTouch(projectID, flagKey string, now time.Time) bool {
	key := projectID + "\x00" + flagKey
	svc.touchedMu.Lock()
	defer svc.touchedMu.Unlock()
	if last, ok := svc.touchedAt[key]; ok && now.Sub(last) < touchInterval {
		return false
	}
	if len(svc.touchedAt) >= maxTrackedTouches {
		svc.touchedAt = make(map[string]time.Time, maxTrackedTouches)
	}
	svc.touchedAt[key] = now
	return true
}

// touchEvaluated best-effort bumps last_evaluated_at for any flag, whatever its
// origin or evaluation reason, off the request path so it never adds latency or
// fails the evaluation.
func (svc *FlagService) touchEvaluated(projectID, flagKey string) {
	if !svc.shouldTouch(projectID, flagKey, time.Now()) {
		return
	}
	bblog.Go("flags-touch-evaluated", func() {
		ctx := context.Background()
		f, err := svc.store.FlagByKey(ctx, projectID, flagKey)
		if err != nil {
			return
		}
		if err := svc.store.TouchFlagEvaluated(ctx, f.ID); err != nil {
			slog.WarnContext(ctx, "flag: touch last_evaluated_at", "err", err, "handled", true,
				"flag_id", f.ID, "project_id", projectID)
		}
	})
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
			tracing.RecordError(span, err)
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

func (svc *FlagService) ContextKeySuggestions(ctx context.Context, projectID string) ([]repository.ContextKeySuggestion, error) {
	return svc.store.FlagContextKeySuggestions(ctx, projectID)
}
