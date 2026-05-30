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
