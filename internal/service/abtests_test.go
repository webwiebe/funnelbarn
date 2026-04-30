package service_test

import (
	"math"
	"testing"

	"github.com/wiebe-xyz/funnelbarn/internal/service"
)

func TestZTest_SignificantResult(t *testing.T) {
	// Large samples with a clear difference — expect significance.
	z, sig := service.ZTest(1000, 200, 1000, 300)
	if !sig {
		t.Errorf("expected significant result, z=%.4f", z)
	}
	if z < 1.96 {
		t.Errorf("z should be > 1.96 for significant result, got %.4f", z)
	}
}

func TestZTest_NotSignificant(t *testing.T) {
	// Tiny samples with no difference — expect non-significant.
	z, sig := service.ZTest(10, 5, 10, 5)
	if sig {
		t.Errorf("unexpected significance: z=%.4f", z)
	}
	_ = z
}

func TestZTest_ZeroSamples(t *testing.T) {
	z, sig := service.ZTest(0, 0, 1000, 200)
	if z != 0 || sig {
		t.Errorf("zero n1: want z=0, sig=false, got z=%.4f, sig=%v", z, sig)
	}

	z, sig = service.ZTest(1000, 200, 0, 0)
	if z != 0 || sig {
		t.Errorf("zero n2: want z=0, sig=false, got z=%.4f, sig=%v", z, sig)
	}
}

func TestZTest_PerfectConvergence(t *testing.T) {
	// Identical conversion rates — z should be 0 or very small.
	z, sig := service.ZTest(1000, 500, 1000, 500)
	if sig {
		t.Errorf("identical rates should not be significant, z=%.4f", z)
	}
	if math.Abs(z) > 0.001 {
		t.Errorf("z should be ~0 for identical rates, got %.4f", z)
	}
}

func TestZTest_AllConversions(t *testing.T) {
	// pPool = 1 → denominator goes to 0 → should return (0, false).
	z, sig := service.ZTest(100, 100, 100, 100)
	if sig {
		t.Errorf("all-converted: should not be significant")
	}
	if z != 0 {
		t.Errorf("all-converted: z should be 0, got %.4f", z)
	}
}

func TestZTest_Threshold(t *testing.T) {
	// The threshold is exactly 1.96 for 95% CI.
	// z=1.97 → significant; z=1.95 → not.
	// Manufacture inputs that produce a known z.
	// Rather than reverse-engineering exact inputs, trust the property:
	// significant iff |z| > 1.96.
	z, sig := service.ZTest(10000, 200, 10000, 220)
	if sig != (z > 1.96) {
		t.Errorf("significant (%v) does not match z=%.4f > 1.96", sig, z)
	}
}
