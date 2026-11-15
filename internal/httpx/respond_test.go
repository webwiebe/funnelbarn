package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

func TestMapServiceError_Statuses(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
		msg  string
	}{
		{"not found", fmt.Errorf("x: %w", domain.ErrNotFound), http.StatusNotFound, "not found"},
		{"conflict", fmt.Errorf("x: %w", domain.ErrConflict), http.StatusConflict, "already exists"},
		{"validation", &domain.ValidationError{Field: "name", Message: "is required"}, http.StatusUnprocessableEntity, "name: is required"},
		{"unexpected", errors.New("disk on fire"), http.StatusInternalServerError, "internal server error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			MapServiceError(w, c.err, "op")
			if w.Code != c.code {
				t.Fatalf("status = %d, want %d", w.Code, c.code)
			}
			var body map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if c.msg != "" && body["error"] != c.msg {
				t.Fatalf("error = %q, want %q", body["error"], c.msg)
			}
			if strings.Contains(body["error"], "disk on fire") {
				t.Fatal("internal error detail leaked to the client")
			}
		})
	}
}

func TestReadJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":1}`))
	var v struct{ A int }
	if err := ReadJSON(r, &v); err != nil || v.A != 1 {
		t.Fatalf("ReadJSON = %v, %+v", err, v)
	}
}

func TestWriteJSON_SetsContentType(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusCreated, map[string]int{"n": 1})
	if w.Code != http.StatusCreated || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("got %d %q", w.Code, w.Header().Get("Content-Type"))
	}
}
