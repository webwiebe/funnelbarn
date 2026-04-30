package domain

import "errors"

// Sentinel errors that the service layer returns.
// Handlers map these to HTTP status codes.
var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("already exists")
	ErrForbidden  = errors.New("forbidden")
	ErrValidation = errors.New("validation error")
)

// ValidationError wraps a field-level validation message.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Message
	}
	return e.Message
}

func (e *ValidationError) Unwrap() error { return ErrValidation }

// IsNotFound reports whether err is or wraps ErrNotFound.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// IsConflict reports whether err is or wraps ErrConflict.
func IsConflict(err error) bool { return errors.Is(err, ErrConflict) }

// IsValidation reports whether err is or wraps ErrValidation.
func IsValidation(err error) bool { return errors.Is(err, ErrValidation) }
