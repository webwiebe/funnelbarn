package environment

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"production", Production},
		{"prod", Production},
		{"prd", Production},
		{"live", Production},
		{"PROD", Production},
		{"staging", Staging},
		{"stg", Staging},
		{"stage", Staging},
		{"acceptance", Staging},
		{"acc", Staging},
		{"test", Test},
		{"testing", Test},
		{"tst", Test},
		{"qa", Test},
		{"uat", Test},
		{"pr", Test},
		{"development", Development},
		{"dev", Development},
		{"local", Development},
		{"develop", Development},
		// Unknown input → production
		{"unknown-env", Production},
		{"feat-branch", Production},
	}

	for _, tc := range tests {
		got := Normalize(tc.input)
		if got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestIsKnown(t *testing.T) {
	known := []string{"", "prod", "PROD", " staging ", "qa", "local"}
	for _, s := range known {
		if !IsKnown(s) {
			t.Errorf("IsKnown(%q) = false, want true", s)
		}
	}
	// These still normalise to production, but the caller can now say so.
	for _, s := range []string{"prodution", "feat-branch", "not-a-real-env"} {
		if IsKnown(s) {
			t.Errorf("IsKnown(%q) = true, want false", s)
		}
		if got := Normalize(s); got != Production {
			t.Errorf("Normalize(%q) = %q, want %q — a typo must still be filed, not dropped", s, got, Production)
		}
	}
}

func TestCanonical(t *testing.T) {
	got := Canonical()
	if len(got) != 4 || got[0] != Production {
		t.Errorf("Canonical() = %v", got)
	}
	for _, e := range got {
		if Normalize(e) != e {
			t.Errorf("canonical value %q does not normalise to itself", e)
		}
	}
}
