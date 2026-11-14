package api

import (
	"testing"
)

func TestParseFlowsDepth(t *testing.T) {
	cases := map[string]int{
		"":    5,
		"1":   1,
		"10":  10,
		"0":   5,
		"11":  5,
		"3x":  3,
		"1a0": 10,
		"-4":  4,
		"abc": 5,
	}
	for in, want := range cases {
		if got := parseFlowsDepth(in); got != want {
			t.Errorf("parseFlowsDepth(%q) = %d, want %d", in, got, want)
		}
	}
}
