package api

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

// mapServiceError maps domain/service errors to appropriate HTTP status codes.
// It never leaks internal error details to the client.
// Expected errors (not-found, conflict, validation) are logged at Warn level with handled=true.
// A request the client cancelled (context.Canceled) is logged at Debug and
// answered 499: nobody is left to read it and nothing failed on our side.
// Unexpected errors are logged at Error level with handled=false.
func mapServiceError(w http.ResponseWriter, err error, op string) {
	switch {
	case domain.IsNotFound(err):
		slog.Warn("service error: not found", "op", op, "error", err, "handled", true)
		jsonError(w, "not found", http.StatusNotFound)
	case domain.IsConflict(err):
		slog.Warn("service error: conflict", "op", op, "error", err, "handled", true)
		jsonError(w, "already exists", http.StatusConflict)
	case domain.IsValidation(err):
		slog.Warn("service error: validation", "op", op, "error", err, "handled", true)
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			jsonError(w, ve.Error(), http.StatusUnprocessableEntity)
		} else {
			jsonError(w, "invalid request", http.StatusUnprocessableEntity)
		}
	case errors.Is(err, context.Canceled):
		slog.Debug("service error: client closed the request", "op", op, "error", err, "handled", true)
		jsonError(w, "client closed request", statusClientClosedRequest)
	default:
		slog.Error("unexpected service error", "op", op, "error", err, "handled", false)
		jsonError(w, "internal server error", http.StatusInternalServerError)
	}
}

// --------------------------------------------------------------------------
// Helper utilities
// --------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json response", "err", err)
	}
}

func jsonError(w http.ResponseWriter, msg string, status int) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}
