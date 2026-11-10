package service

import (
	"errors"
	"log/slog"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// EvalErrorCode is the error_code of a flag evaluation that came back as
// reason ERROR. The wire values follow OpenFeature where it has one.
type EvalErrorCode string

const (
	// EvalFlagNotFound: the key names no flag. Normal client behaviour, and
	// the path auto-registration is built on.
	EvalFlagNotFound EvalErrorCode = "FLAG_NOT_FOUND"
	// EvalAutoRegisterLimit: the project reached its cap on auto-created flags.
	EvalAutoRegisterLimit EvalErrorCode = "AUTO_REGISTER_LIMIT"
	// EvalVariantNotFound: a stored targeting rule names a variant the flag
	// does not define. A configuration defect in the stored flag.
	EvalVariantNotFound EvalErrorCode = "VARIANT_NOT_FOUND"
	// EvalGeneral: anything else, such as a database failure.
	EvalGeneral EvalErrorCode = "GENERAL"
)

// EvalErrorCodeOf classifies an evaluation error by type, never by its text.
func EvalErrorCodeOf(err error) EvalErrorCode {
	switch {
	case errors.Is(err, domain.ErrAutoRegisterLimit):
		return EvalAutoRegisterLimit
	case errors.Is(err, domain.ErrFlagNotFound):
		return EvalFlagNotFound
	default:
		return EvalGeneral
	}
}

// IsFault reports whether the code marks a defect of ours: it is logged at
// Error and recorded as a span error. FLAG_NOT_FOUND and AUTO_REGISTER_LIMIT
// are expected outcomes a client can cause.
func (c EvalErrorCode) IsFault() bool {
	return c != EvalFlagNotFound && c != EvalAutoRegisterLimit
}

// LogLevel is the level an evaluation error with this code is logged at.
// A missing flag is routine (Debug); a project at its cap is a possible-abuse
// signal worth a Warn; everything else is a server problem.
func (c EvalErrorCode) LogLevel() slog.Level {
	switch c {
	case EvalFlagNotFound:
		return slog.LevelDebug
	case EvalAutoRegisterLimit:
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
}

// LogMessage is the log line for an evaluation error with this code. The three
// texts stay fixed so existing log queries and BugBarn groups keep matching.
func (c EvalErrorCode) LogMessage() string {
	switch c {
	case EvalFlagNotFound:
		return "flag evaluate: not found"
	case EvalAutoRegisterLimit:
		return "flag auto-register limit reached"
	default:
		return "flag evaluate failed"
	}
}
