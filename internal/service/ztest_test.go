package service_test

import (
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

func TestZTestTwoProportions(t *testing.T) {
	cases := []struct {
		name           string
		n1, x1, n2, x2 int64
		wantZero       bool
		wantSig        bool
	}{
		{"zero sample n1", 0, 0, 100, 50, true, false},
		{"zero sample n2", 100, 50, 0, 0, true, false},
		// Both arms 0% — pooled p is 0, can't compute.
		{"both zero conversions", 100, 0, 100, 0, true, false},
		// Both arms 100% — pooled p is 1, can't compute.
		{"both full conversions", 100, 100, 100, 100, true, false},
		// Equal rates → z = 0, not significant. Pure-zero is the *correct*
		// answer here (numerator of the z stat is p1 - p2 = 0), so allow it
		// alongside other non-significant cases.
		{"equal rates", 1000, 100, 1000, 100, true, false},
		// Strong difference, large samples → significant.
		{"large diff large sample", 1000, 500, 1000, 100, false, true},
		// Small difference, small sample → not significant.
		{"small diff small sample", 50, 25, 50, 22, false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			z, sig := service.ZTestTwoProportions(c.n1, c.x1, c.n2, c.x2)
			if c.wantZero && z != 0 {
				t.Errorf("z: want 0, got %f", z)
			}
			if !c.wantZero && z == 0 {
				t.Error("z: got 0 but expected a non-zero value")
			}
			if sig != c.wantSig {
				t.Errorf("significant: want %v, got %v (z=%f)", c.wantSig, sig, z)
			}
		})
	}
}
