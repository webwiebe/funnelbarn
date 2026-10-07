package command

import (
	"context"
	"database/sql"
	"errors"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// RecordEvaluation stores one flag evaluation row.
type RecordEvaluation struct {
	Store interface {
		RecordEvaluation(ctx context.Context, eval repository.FlagEvaluation) error
	}
	Eval repository.FlagEvaluation
}

// Kind implements Command.
func (RecordEvaluation) Kind() string { return "record_evaluation" }

// Apply implements Command.
func (c RecordEvaluation) Apply(ctx context.Context) error {
	return c.Store.RecordEvaluation(ctx, c.Eval)
}

// TouchAPIKey updates last_used_at for an API key.
type TouchAPIKey struct {
	Store interface {
		TouchAPIKey(ctx context.Context, keySHA256 string) error
	}
	KeyHash string
}

// Kind implements Command.
func (TouchAPIKey) Kind() string { return "touch_api_key" }

// Apply implements Command.
func (c TouchAPIKey) Apply(ctx context.Context) error {
	return c.Store.TouchAPIKey(ctx, c.KeyHash)
}

// TouchFlagEvaluated stamps last_evaluated_at on a flag found by key.
type TouchFlagEvaluated struct {
	Store interface {
		FlagByKey(ctx context.Context, projectID, flagKey string) (repository.FeatureFlag, error)
		TouchFlagEvaluated(ctx context.Context, flagID string) error
	}
	ProjectID string
	FlagKey   string
}

// Kind implements Command.
func (TouchFlagEvaluated) Kind() string { return "touch_flag_evaluated" }

// Apply implements Command.
func (c TouchFlagEvaluated) Apply(ctx context.Context) error {
	f, err := c.Store.FlagByKey(ctx, c.ProjectID, c.FlagKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // EnsureAutoFlag skipped it at the cap; nothing to stamp
	}
	if err != nil {
		return err
	}
	return c.Store.TouchFlagEvaluated(ctx, f.ID)
}

// MarkFlagsEvaluated records that a project has called the evaluate endpoint.
type MarkFlagsEvaluated struct {
	Mark      func(ctx context.Context, projectID string) error
	ProjectID string
}

// Kind implements Command.
func (MarkFlagsEvaluated) Kind() string { return "mark_flags_evaluated" }

// Apply implements Command.
func (c MarkFlagsEvaluated) Apply(ctx context.Context) error {
	return c.Mark(ctx, c.ProjectID)
}

// EnsureAutoFlag inserts an auto-registered flag if the key is still unknown.
// It is idempotent per (project, key). The request checked the cap against the
// read pool, but other EnsureAutoFlag commands may still have been queued, so
// with Max > 0 the cap is checked again here, where commands apply one at a
// time, and a new key past the cap is skipped.
type EnsureAutoFlag struct {
	Store interface {
		EnsureAutoFlag(ctx context.Context, f repository.FeatureFlag) (repository.FeatureFlag, error)
		CountAutoFlags(ctx context.Context, projectID string) (int, error)
	}
	Flag repository.FeatureFlag
	Max  int
}

// Kind implements Command.
func (EnsureAutoFlag) Kind() string { return "ensure_auto_flag" }

// Apply implements Command.
func (c EnsureAutoFlag) Apply(ctx context.Context) error {
	if c.Max > 0 {
		n, err := c.Store.CountAutoFlags(ctx, c.Flag.ProjectID)
		if err != nil {
			return err
		}
		if n >= c.Max {
			// At the cap. ON CONFLICT DO NOTHING makes an existing key a no-op
			// anyway, so skipping is safe for it too.
			return nil
		}
	}
	_, err := c.Store.EnsureAutoFlag(ctx, c.Flag)
	return err
}
