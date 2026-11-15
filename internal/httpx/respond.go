// Package httpx holds the JSON response helpers every HTTP handler shares:
// the API server and the feature slices answer through the same error mapping.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

// statusClientClosedRequest is the non-standard 499 that proxies use for a
// client that gave up before the response.
const statusClientClosedRequest = 499

// MapServiceError maps domain/service errors to appropriate HTTP status codes.
// It never leaks internal error details to the client.
// Expected errors (not-found, conflict, validation) are logged at Warn level with handled=true.
// A request the client cancelled (context.Canceled) is logged at Debug and
// answered 499: nobody is left to read it and nothing failed on our side.
// Unexpected errors are logged at Error level with handled=false.
func MapServiceError(w http.ResponseWriter, err error, op string) {
	switch {
	case domain.IsNotFound(err):
		slog.Warn("service error: not found", "op", op, "error", err, "handled", true)
		JSONError(w, "not found", http.StatusNotFound)
	case domain.IsConflict(err):
		slog.Warn("service error: conflict", "op", op, "error", err, "handled", true)
		JSONError(w, "already exists", http.StatusConflict)
	case domain.IsValidation(err):
		slog.Warn("service error: validation", "op", op, "error", err, "handled", true)
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			JSONError(w, ve.Error(), http.StatusUnprocessableEntity)
		} else {
			JSONError(w, "invalid request", http.StatusUnprocessableEntity)
		}
	case errors.Is(err, context.Canceled):
		slog.Debug("service error: client closed the request", "op", op, "error", err, "handled", true)
		JSONError(w, "client closed request", statusClientClosedRequest)
	default:
		slog.Error("unexpected service error", "op", op, "error", err, "handled", false)
		JSONError(w, "internal server error", http.StatusInternalServerError)
	}
}

// WriteJSON writes v as a JSON response with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json response", "err", err)
	}
}

// JSONError writes {"error": msg} with the given status.
func JSONError(w http.ResponseWriter, msg string, status int) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// ReadJSON decodes the request body into v.
func ReadJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
