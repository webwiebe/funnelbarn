package domain_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/domain"
)

func TestIsNotFound(t *testing.T) {
	t.Run("direct sentinel", func(t *testing.T) {
		if !domain.IsNotFound(domain.ErrNotFound) {
			t.Error("expected IsNotFound(ErrNotFound) to be true")
		}
	})

	t.Run("wrapped sentinel", func(t *testing.T) {
		wrapped := fmt.Errorf("project abc: %w", domain.ErrNotFound)
		if !domain.IsNotFound(wrapped) {
			t.Error("expected IsNotFound(wrapped ErrNotFound) to be true")
		}
	})

	t.Run("unrelated error", func(t *testing.T) {
		if domain.IsNotFound(errors.New("something else")) {
			t.Error("expected IsNotFound to be false for unrelated error")
		}
	})
}

func TestIsConflict(t *testing.T) {
	t.Run("direct sentinel", func(t *testing.T) {
		if !domain.IsConflict(domain.ErrConflict) {
			t.Error("expected IsConflict(ErrConflict) to be true")
		}
	})

	t.Run("wrapped sentinel", func(t *testing.T) {
		wrapped := fmt.Errorf("slug %q: %w", "my-slug", domain.ErrConflict)
		if !domain.IsConflict(wrapped) {
			t.Error("expected IsConflict(wrapped ErrConflict) to be true")
		}
	})

	t.Run("unrelated error", func(t *testing.T) {
		if domain.IsConflict(domain.ErrNotFound) {
			t.Error("expected IsConflict to be false for ErrNotFound")
		}
	})
}

func TestIsValidation(t *testing.T) {
	t.Run("direct sentinel", func(t *testing.T) {
		if !domain.IsValidation(domain.ErrValidation) {
			t.Error("expected IsValidation(ErrValidation) to be true")
		}
	})

	t.Run("ValidationError unwraps to ErrValidation", func(t *testing.T) {
		ve := &domain.ValidationError{Field: "name", Message: "required"}
		if !domain.IsValidation(ve) {
			t.Error("expected IsValidation(ValidationError) to be true")
		}
	})

	t.Run("unrelated error", func(t *testing.T) {
		if domain.IsValidation(domain.ErrNotFound) {
			t.Error("expected IsValidation to be false for ErrNotFound")
		}
	})
}

func TestValidationError_Error(t *testing.T) {
	t.Run("with field", func(t *testing.T) {
		ve := &domain.ValidationError{Field: "name", Message: "required"}
		want := "name: required"
		if got := ve.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("without field", func(t *testing.T) {
		ve := &domain.ValidationError{Message: "something invalid"}
		want := "something invalid"
		if got := ve.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})
}

func TestValidationError_Unwrap(t *testing.T) {
	ve := &domain.ValidationError{Field: "slug", Message: "required"}
	if !errors.Is(ve, domain.ErrValidation) {
		t.Error("expected ValidationError to unwrap to ErrValidation")
	}
}

func TestWrappedErrors(t *testing.T) {
	// Test that wrapping domain errors preserves Is() behaviour
	wrapped := fmt.Errorf("op failed: %w", domain.ErrNotFound)
	if !domain.IsNotFound(wrapped) {
		t.Error("wrapped ErrNotFound not detected by IsNotFound")
	}

	wrapped2 := fmt.Errorf("op: %w", domain.ErrConflict)
	if !domain.IsConflict(wrapped2) {
		t.Error("wrapped ErrConflict not detected by IsConflict")
	}
}

func TestValidationError_FieldEmpty(t *testing.T) {
	err := &domain.ValidationError{Message: "required"}
	if err.Error() != "required" {
		t.Errorf("want 'required', got %q", err.Error())
	}
}
