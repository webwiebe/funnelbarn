package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// errNotFound is returned for objects that do not exist or belong to another
// project than the resolved one. The two cases read the same on purpose.
var errNotFound = domain.ErrNotFound

// inputError is a problem with the tool's arguments. Its message is shown to
// the assistant as is.
type inputError struct{ msg string }

func (e *inputError) Error() string { return e.msg }

// invalidInput returns an error for bad tool arguments.
func invalidInput(format string, args ...any) error {
	return &inputError{msg: fmt.Sprintf(format, args...)}
}

// toolError maps an error from a tool handler to the message the assistant
// sees. Argument, validation, not-found and conflict errors keep a readable
// message and log at Warn. Anything else is unexpected: it logs at Error (so
// selflog reports it to BugBarn) and the client only sees "internal error",
// so raw database errors never leave the server.
func toolError(ctx context.Context, log *slog.Logger, tool string, err error) error {
	var in *inputError
	var ve *domain.ValidationError
	switch {
	case errors.As(err, &in):
		log.WarnContext(ctx, "mcp tool invalid input", "tool", tool, "err", err)
		return err
	case errors.As(err, &ve):
		log.WarnContext(ctx, "mcp tool validation failed", "tool", tool, "err", err)
		return errors.New(ve.Error())
	case domain.IsValidation(err):
		log.WarnContext(ctx, "mcp tool validation failed", "tool", tool, "err", err)
		return err
	case domain.IsNotFound(err):
		log.WarnContext(ctx, "mcp tool not found", "tool", tool, "err", err)
		return errors.New("not found")
	case domain.IsConflict(err):
		log.WarnContext(ctx, "mcp tool conflict", "tool", tool, "err", err)
		return errors.New("already exists")
	case errors.Is(err, domain.ErrAutoRegisterLimit):
		return errors.New("auto-register limit reached for this project")
	case errors.Is(err, domain.ErrForbidden):
		log.WarnContext(ctx, "mcp tool forbidden", "tool", tool, "err", err)
		return errors.New("forbidden")
	case errors.Is(err, context.Canceled):
		return errors.New("request canceled")
	default:
		log.ErrorContext(ctx, "mcp tool failed", "tool", tool, "err", err)
		return errors.New("internal error")
	}
}
