// Package middleware provides HTTP middleware functions shared across the
// api layer.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type ctxKey string

// RequestIDKey is the context key for the request ID.
// Exported so that bblog and other packages can read it without creating an
// import cycle.
const RequestIDKey ctxKey = "request_id"

// RequestID attaches a unique request ID to every request.
//
// The ID is taken from the incoming X-Request-ID header if present (proxy
// forwarding), otherwise a random 16-character hex string is generated.
// The resolved ID is written back on the X-Request-ID response header so
// clients can correlate logs.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newID()
		}
		ctx := context.WithValue(r.Context(), RequestIDKey, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FromContext extracts the request ID from ctx, or returns "" if absent.
func FromContext(ctx context.Context) string {
	v, _ := ctx.Value(RequestIDKey).(string)
	return v
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
