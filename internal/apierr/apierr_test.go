package apierr_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/apierr"
)

func TestNamedConstructors(t *testing.T) {
	cases := []struct {
		name string
		err  *apierr.Error
		code int
	}{
		{"NotFound", apierr.NotFound("gone"), http.StatusNotFound},
		{"BadRequest", apierr.BadRequest("bad"), http.StatusBadRequest},
		{"Unauthorized", apierr.Unauthorized("no"), http.StatusUnauthorized},
		{"Forbidden", apierr.Forbidden("no"), http.StatusForbidden},
		{"Conflict", apierr.Conflict("dup"), http.StatusConflict},
		{"TooManyRequests", apierr.TooManyRequests("slow"), http.StatusTooManyRequests},
		{"Internal", apierr.Internal(), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Code != tc.code {
				t.Errorf("Code: want %d, got %d", tc.code, tc.err.Code)
			}
			if tc.err.Error() != tc.err.Message {
				t.Errorf("Error() should return Message")
			}
		})
	}
}

func TestMapDB_NoRows(t *testing.T) {
	e := apierr.MapDB(sql.ErrNoRows, "thing not found")
	if e == nil {
		t.Fatal("expected non-nil error")
	}
	if e.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", e.Code)
	}
	if e.Message != "thing not found" {
		t.Errorf("unexpected message: %q", e.Message)
	}
}

func TestMapDB_OtherError(t *testing.T) {
	e := apierr.MapDB(sql.ErrConnDone, "thing not found")
	if e.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", e.Code)
	}
	// Must not expose the internal sql error message.
	if e.Message != "internal server error" {
		t.Errorf("want safe message, got %q", e.Message)
	}
}

func TestMapDB_Nil(t *testing.T) {
	if apierr.MapDB(nil, "x") != nil {
		t.Error("MapDB(nil) should return nil")
	}
}

func TestWriteHTTP_KnownError(t *testing.T) {
	w := httptest.NewRecorder()
	apierr.WriteHTTP(w, apierr.NotFound("widget not found"))

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("want application/json, got %q", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not valid JSON: %v", err)
	}
	if body["error"] != "widget not found" {
		t.Errorf("unexpected error message: %q", body["error"])
	}
}

func TestWriteHTTP_UnknownError(t *testing.T) {
	w := httptest.NewRecorder()
	apierr.WriteHTTP(w, sql.ErrConnDone) // not an *apierr.Error

	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", w.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	// Must not expose the internal message.
	if body["error"] != "internal server error" {
		t.Errorf("sensitive error leaked: %q", body["error"])
	}
}

func TestWriteHTTP_Nil(t *testing.T) {
	w := httptest.NewRecorder()
	apierr.WriteHTTP(w, nil)
	if w.Code != http.StatusOK {
		t.Errorf("WriteHTTP(nil) should write nothing (status stays 200)")
	}
}
