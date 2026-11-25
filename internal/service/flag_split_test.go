package service

import "testing"

func TestParseSplitSpec(t *testing.T) {
	got, err := ParseSplitSpec("true=50, false=50")
	if err != nil || got != `{"false":50,"true":50}` {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, bad := range []string{"", "true", "true=x", "=50", "true=50,"} {
		if _, err := ParseSplitSpec(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestValidateSplit(t *testing.T) {
	variants := `{"true":true,"false":false}`
	cases := []struct {
		name, split string
		ok          bool
	}{
		{"even", `{"true":50,"false":50}`, true},
		{"all one", `{"true":100}`, true},
		{"short", `{"true":40,"false":50}`, false},
		{"over", `{"true":60,"false":50}`, false},
		{"unknown variant", `{"true":50,"maybe":50}`, false},
		{"negative", `{"true":-10,"false":110}`, false},
		{"not json", `x`, false},
	}
	for _, c := range cases {
		if err := ValidateSplit(variants, c.split); (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}
