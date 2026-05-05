// Package apierr defines safe, user-visible API error types and their HTTP
// status codes. Each layer uses these instead of raw database or stdlib errors:
//
//   - Repositories surface sql.ErrNoRows → convert with MapDB at the handler boundary.
//   - Handlers call WriteHTTP(w, apierr.MapDB(err, "thing not found")) rather than
//     checking errors.Is(err, sql.ErrNoRows) inline.
//   - Sensitive details (SQL messages, stack traces) never reach the API consumer.
package apierr

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
)

// Error is a safe, user-visible error with an HTTP status code.
// It never exposes internal details like SQL messages or stack traces.
type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string { return e.Message }

// Named constructors — use these instead of &Error{} literals.

func NotFound(msg string) *Error     { return &Error{Code: http.StatusNotFound, Message: msg} }
func BadRequest(msg string) *Error   { return &Error{Code: http.StatusBadRequest, Message: msg} }
func Unauthorized(msg string) *Error { return &Error{Code: http.StatusUnauthorized, Message: msg} }
func Forbidden(msg string) *Error    { return &Error{Code: http.StatusForbidden, Message: msg} }
func Conflict(msg string) *Error     { return &Error{Code: http.StatusConflict, Message: msg} }
func TooManyRequests(msg string) *Error {
	return &Error{Code: http.StatusTooManyRequests, Message: msg}
}
func Internal() *Error {
	return &Error{Code: http.StatusInternalServerError, Message: "internal server error"}
}

// MapDB converts a database-layer error to an *Error.
// sql.ErrNoRows → 404 with notFoundMsg.
// nil → nil (pass-through for the happy path).
// Anything else → 500 "internal server error" (safe: hides DB details).
func MapDB(err error, notFoundMsg string) *Error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return NotFound(notFoundMsg)
	}
	return Internal()
}

// WriteHTTP writes err as a JSON error response.
// If err is an *Error, its Code and Message are used.
// Any other non-nil error becomes a 500.
func WriteHTTP(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	code := http.StatusInternalServerError
	msg := "internal server error"
	var e *Error
	if errors.As(err, &e) {
		code = e.Code
		msg = e.Message
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
