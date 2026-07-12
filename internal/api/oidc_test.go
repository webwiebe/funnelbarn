package api

import (
	"testing"
)

func TestParseStateCookie(t *testing.T) {
	tests := []struct {
		in           string
		state, verif string
		ok           bool
	}{
		{"abc|def", "abc", "def", true},
		{"nopipe", "", "", false},
		{"|def", "", "", false},
		{"abc|", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range tests {
		state, verif, ok := parseStateCookie(tc.in)
		if ok != tc.ok || state != tc.state || verif != tc.verif {
			t.Errorf("parseStateCookie(%q) = (%q,%q,%v), want (%q,%q,%v)",
				tc.in, state, verif, ok, tc.state, tc.verif, tc.ok)
		}
	}
}
