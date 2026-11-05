package worker

import (
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/wiebe-xyz/funnelbarn/internal/metrics"
	"github.com/wiebe-xyz/funnelbarn/internal/session"
)

func TestResolveSessionID(t *testing.T) {
	in := session.FingerprintInput{ClientIP: "203.0.113.7", UserAgent: "ua", ProjectSlug: "p",
		Environment: "production", At: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)}
	fingerprint := session.Fingerprint(in)
	valid := "0123456789abcdef0123456789abcdef"
	if !session.IsValidSessionID(valid) {
		t.Fatalf("test fixture %q is not a valid session ID", valid)
	}

	cases := []struct {
		name, sent, want, reason string
	}{
		{name: "valid id is kept", sent: valid, want: valid},
		{name: "missing id is fingerprinted", sent: "", want: fingerprint, reason: "missing"},
		{name: "invalid id is fingerprinted", sent: "not-one-of-ours", want: fingerprint, reason: "invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			missing := fingerprinted(t, "missing")
			invalid := fingerprinted(t, "invalid")

			if got := resolveSessionID(tc.sent, in); got != tc.want {
				t.Errorf("resolveSessionID(%q) = %q, want %q", tc.sent, got, tc.want)
			}

			wantMissing, wantInvalid := missing, invalid
			switch tc.reason {
			case "missing":
				wantMissing++
			case "invalid":
				wantInvalid++
			}
			if got := fingerprinted(t, "missing"); got != wantMissing {
				t.Errorf("missing counter = %v, want %v", got, wantMissing)
			}
			if got := fingerprinted(t, "invalid"); got != wantInvalid {
				t.Errorf("invalid counter = %v, want %v", got, wantInvalid)
			}
		})
	}
}

// fingerprinted reads the SessionsFingerprinted counter for one reason.
func fingerprinted(t *testing.T, reason string) float64 {
	t.Helper()
	var m dto.Metric
	if err := metrics.SessionsFingerprinted.WithLabelValues(reason).Write(&m); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	return m.GetCounter().GetValue()
}
