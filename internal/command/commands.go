package command

import (
	"context"
	"database/sql"
	"errors"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// Kinds of the bookkeeping commands. They label metrics and spans and select
// the payload type when an Envelope is decoded.
const (
	KindRecordEvaluation   = "record_evaluation"
	KindTouchAPIKey        = "touch_api_key"
	KindTouchFlagEvaluated = "touch_flag_evaluated"
	KindMarkFlagsEvaluated = "mark_flags_evaluated"
	KindEnsureAutoFlag     = "ensure_auto_flag"
)

// RecordEvaluation stores one flag evaluation row. Eval.ID makes the insert
// idempotent: Encode assigns one when it is empty, so a redelivered command
// inserts nothing new.
type RecordEvaluation struct {
	Eval repository.FlagEvaluation `json:"eval"`
}

// Kind implements Command.
func (RecordEvaluation) Kind() string { return KindRecordEvaluation }

// Project implements Command.
func (c RecordEvaluation) Project() string { return c.Eval.ProjectID }

// Apply implements Command.
func (c RecordEvaluation) Apply(ctx context.Context, d Deps) error {
	return d.Store.RecordEvaluation(ctx, c.Eval)
}

// TouchAPIKey updates last_used_at for an API key.
type TouchAPIKey struct {
	KeyHash string `json:"key_hash"`
}

// Kind implements Command.
func (TouchAPIKey) Kind() string { return KindTouchAPIKey }

// Project implements Command. A key hash names no project.
func (TouchAPIKey) Project() string { return "" }

// Apply implements Command.
func (c TouchAPIKey) Apply(ctx context.Context, d Deps) error {
	return d.Store.TouchAPIKey(ctx, c.KeyHash)
}

// TouchFlagEvaluated stamps last_evaluated_at on a flag found by key.
type TouchFlagEvaluated struct {
	ProjectID string `json:"project_id"`
	FlagKey   string `json:"flag_key"`
}

// Kind implements Command.
func (TouchFlagEvaluated) Kind() string { return KindTouchFlagEvaluated }

// Project implements Command.
func (c TouchFlagEvaluated) Project() string { return c.ProjectID }

// Apply implements Command.
func (c TouchFlagEvaluated) Apply(ctx context.Context, d Deps) error {
	f, err := d.Store.FlagByKey(ctx, c.ProjectID, c.FlagKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // EnsureAutoFlag skipped it at the cap; nothing to stamp
	}
	if err != nil {
		return err
	}
	return d.Store.TouchFlagEvaluated(ctx, f.ID)
}

// MarkFlagsEvaluated records that a project has called the evaluate endpoint.
type MarkFlagsEvaluated struct {
	ProjectID string `json:"project_id"`
}

// Kind implements Command.
func (MarkFlagsEvaluated) Kind() string { return KindMarkFlagsEvaluated }

// Project implements Command.
func (c MarkFlagsEvaluated) Project() string { return c.ProjectID }

// Apply implements Command.
func (c MarkFlagsEvaluated) Apply(ctx context.Context, d Deps) error {
	return d.MarkFlagsEvaluated(ctx, c.ProjectID)
}

// EnsureAutoFlag inserts an auto-registered flag if the key is still unknown.
// It is idempotent per (project, key). The request checked the cap against the
// read pool, but other EnsureAutoFlag commands may still have been queued, so
// with Max > 0 the cap is checked again here, where commands apply one at a
// time, and a new key past the cap is skipped.
type EnsureAutoFlag struct {
	Flag repository.FeatureFlag `json:"flag"`
	Max  int                    `json:"max"`
}

// Kind implements Command.
func (EnsureAutoFlag) Kind() string { return KindEnsureAutoFlag }

// Project implements Command.
func (c EnsureAutoFlag) Project() string { return c.Flag.ProjectID }

// Apply implements Command.
func (c EnsureAutoFlag) Apply(ctx context.Context, d Deps) error {
	if c.Max > 0 {
		n, err := d.Store.CountAutoFlags(ctx, c.Flag.ProjectID)
		if err != nil {
			return err
		}
		if n >= c.Max {
			// At the cap. ON CONFLICT DO NOTHING makes an existing key a no-op
			// anyway, so skipping is safe for it too.
			return nil
		}
	}
	_, err := d.Store.EnsureAutoFlag(ctx, c.Flag)
	return err
}
