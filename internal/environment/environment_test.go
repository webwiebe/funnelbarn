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

func TestNormalizeEvent(t *testing.T) {
	// An event with no environment is filed as production. Deployment config
	// keeps "" (see TestNormalize) because the API branches on the unset state;
	// an event has no such state and a blank one is invisible to every filter.
	if got := NormalizeEvent(""); got != Production {
		t.Errorf("NormalizeEvent(%q) = %q, want %q", "", got, Production)
	}
	if got := NormalizeEvent("   "); got != Production {
		t.Errorf("NormalizeEvent(%q) = %q, want %q", "   ", got, Production)
	}
	// Everything else behaves exactly as Normalize does.
	for _, s := range []string{"prod", "STG", "qa", "local", "feat-branch", "staging"} {
		if got, want := NormalizeEvent(s), Normalize(s); got != want {
			t.Errorf("NormalizeEvent(%q) = %q, want %q", s, got, want)
		}
	}
	// And it always lands on a canonical value.
	for _, s := range []string{"", "prod", "acc", "uat", "develop", "nonsense"} {
		got := NormalizeEvent(s)
		if Normalize(got) != got {
			t.Errorf("NormalizeEvent(%q) = %q, which is not canonical", s, got)
		}
	}
}
