package service

import "math"

// ZTestTwoProportions performs a two-proportion z-test. It returns the
// z-score and whether the difference is significant at the 95% level
// (|z| > 1.96). The A/B test view and flag analysis (REST and MCP) share it.
func ZTestTwoProportions(n1, x1, n2, x2 int64) (zScore float64, significant bool) {
	if n1 == 0 || n2 == 0 {
		return 0, false
	}
	p1 := float64(x1) / float64(n1)
	p2 := float64(x2) / float64(n2)
	pPool := float64(x1+x2) / float64(n1+n2)
	if pPool == 0 || pPool == 1 {
		return 0, false
	}
	se := math.Sqrt(pPool * (1 - pPool) * (1/float64(n1) + 1/float64(n2)))
	if se == 0 {
		return 0, false
	}
	z := math.Abs((p1 - p2) / se)
	return z, z > 1.96
}
