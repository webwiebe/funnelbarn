package session

import (
	"testing"
	"time"
)

var fixedTime = time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

// fp builds a fingerprint from the common case, letting each test vary only
// the field it cares about.
func fp(mutate ...func(*FingerprintInput)) string {
	in := FingerprintInput{
		ClientIP:    "192.168.1.42:54321",
		UserAgent:   "Mozilla/5.0 Chrome/124",
		ProjectSlug: "my-site",
		Environment: "production",
		At:          fixedTime,
	}
	for _, m := range mutate {
		m(&in)
	}
	return Fingerprint(in)
}

func ip(v string) func(*FingerprintInput)    { return func(i *FingerprintInput) { i.ClientIP = v } }
func ua(v string) func(*FingerprintInput)    { return func(i *FingerprintInput) { i.UserAgent = v } }
func at(v time.Time) func(*FingerprintInput) { return func(i *FingerprintInput) { i.At = v } }

// ---------------------------------------------------------------------------
// Fingerprint
// ---------------------------------------------------------------------------

func TestFingerprint_ReturnsHex32(t *testing.T) {
	got := fp()
	if len(got) != 32 {
		t.Errorf("expected 32 hex chars, got %d: %q", len(got), got)
	}
	if !isHex(got) {
		t.Errorf("fingerprint is not hex: %q", got)
	}
}

func TestFingerprint_Deterministic(t *testing.T) {
	if a, b := fp(), fp(); a != b {
		t.Errorf("Fingerprint not deterministic: %q != %q", a, b)
	}
}

func TestFingerprint_DifferentInputsDifferentOutput(t *testing.T) {
	if a, b := fp(ua("ua-1")), fp(ua("ua-2")); a == b {
		t.Error("different UAs should produce different fingerprints")
	}
}

func TestFingerprint_IsValidSessionID(t *testing.T) {
	if got := fp(ip("203.0.113.10:12345")); !IsValidSessionID(got) {
		t.Errorf("Fingerprint result %q should be a valid session ID", got)
	}
}

// ---------------------------------------------------------------------------
// Inputs that must keep sessions apart
// ---------------------------------------------------------------------------

func TestFingerprint_DifferentEnvironmentsDifferentOutput(t *testing.T) {
	prod := fp()
	dev := fp(func(i *FingerprintInput) { i.Environment = "development" })
	if prod == dev {
		t.Error("different environments should produce different fingerprints")
	}
}

func TestFingerprint_DifferentProjectsDifferentOutput(t *testing.T) {
	// sessions.id is a global primary key, so two projects reached through the
	// same ingress must not mint the same session ID.
	a := fp(func(i *FingerprintInput) { i.ProjectSlug = "site-one" })
	b := fp(func(i *FingerprintInput) { i.ProjectSlug = "site-two" })
	if a == b {
		t.Error("different projects should produce different fingerprints")
	}
}

// ---------------------------------------------------------------------------
// Time window — a fingerprinted session must not grow without bound (#226)
// ---------------------------------------------------------------------------

func TestFingerprint_StableWithinWindow(t *testing.T) {
	// 09:00 and 09:29 fall in the same half-hour window.
	base := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	if a, b := fp(at(base)), fp(at(base.Add(29*time.Minute))); a != b {
		t.Errorf("events inside one window should share a session: %q != %q", a, b)
	}
}

func TestFingerprint_RotatesAcrossWindows(t *testing.T) {
	base := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	for _, gap := range []time.Duration{FingerprintWindow, time.Hour, 24 * time.Hour, 90 * 24 * time.Hour} {
		if a, b := fp(at(base)), fp(at(base.Add(gap))); a == b {
			t.Errorf("events %s apart should not share a session ID", gap)
		}
	}
}

func TestFingerprint_WindowIsAbsoluteNotSliding(t *testing.T) {
	// Continuous activity must still roll over. Stepping a minute at a time
	// across three hours has to produce more than one session ID, or a visitor
	// who never goes idle stays in one session forever.
	base := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	seen := map[string]bool{}
	for i := 0; i <= 180; i++ {
		seen[fp(at(base.Add(time.Duration(i)*time.Minute)))] = true
	}
	if len(seen) < 6 {
		t.Errorf("3 hours of continuous activity produced %d session IDs, want at least 6", len(seen))
	}
}

func TestFingerprint_ZeroTimeDoesNotPanic(t *testing.T) {
	if got := fp(at(time.Time{})); len(got) != 32 {
		t.Errorf("zero timestamp: expected 32 chars, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// IPv4 /24 anonymization
// ---------------------------------------------------------------------------

func TestFingerprint_IPv4AnonymizationSlash24(t *testing.T) {
	// 192.168.1.5 and 192.168.1.99 share /24 prefix → same fingerprint.
	if a, b := fp(ip("192.168.1.5:1000")), fp(ip("192.168.1.99:2000")); a != b {
		t.Errorf("IPv4 /24: same subnet should fingerprint identically, got %q vs %q", a, b)
	}

	// Different /24 subnet → different fingerprint.
	if a, c := fp(ip("192.168.1.5:1000")), fp(ip("192.168.2.5:1000")); a == c {
		t.Error("IPv4: different /24 subnets should produce different fingerprints")
	}
}

func TestFingerprint_IPv4WithoutPort(t *testing.T) {
	// When the address has no port, it should still work.
	if got := fp(ip("10.0.0.1")); len(got) != 32 {
		t.Errorf("expected 32 chars, got %d: %q", len(got), got)
	}
}

func TestFingerprint_IPv4LastOctetIgnored(t *testing.T) {
	base := fp(ip("172.16.0.1:9000"))
	for _, suffix := range []string{"2", "100", "200", "255"} {
		if got := fp(ip("172.16.0." + suffix + ":9000")); got != base {
			t.Errorf("172.16.0.%s should match 172.16.0.1 (same /24), got different fingerprint", suffix)
		}
	}
}

// ---------------------------------------------------------------------------
// IPv6 /48 anonymization
// ---------------------------------------------------------------------------

func TestFingerprint_IPv6AnonymizationSlash48(t *testing.T) {
	// 2001:db8:1234::1 and 2001:db8:1234::2 share /48.
	if a, b := fp(ip("[2001:db8:1234::1]:80")), fp(ip("[2001:db8:1234::2]:80")); a != b {
		t.Errorf("IPv6 /48: same prefix should fingerprint identically, got %q vs %q", a, b)
	}

	// Different /48 prefix → different fingerprint.
	if a, c := fp(ip("[2001:db8:1234::1]:80")), fp(ip("[2001:db8:5678::1]:80")); a == c {
		t.Error("IPv6: different /48 prefixes should produce different fingerprints")
	}
}

func TestFingerprint_IPv6NoBrackets(t *testing.T) {
	if got := fp(ip("::1")); len(got) != 32 {
		t.Errorf("expected 32 chars, got %d: %q", len(got), got)
	}
}

// ---------------------------------------------------------------------------
// normalizeIP
// ---------------------------------------------------------------------------

func TestNormalizeIP_IPv4(t *testing.T) {
	tests := []struct {
		ip   string
		want string
	}{
		{"192.168.1.100", "192.168.1.0"},
		{"10.0.0.255", "10.0.0.0"},
		{"8.8.8.8", "8.8.8.0"},
	}
	for _, tc := range tests {
		if got := normalizeIP(tc.ip); got != tc.want {
			t.Errorf("normalizeIP(%q) = %q, want %q", tc.ip, got, tc.want)
		}
	}
}

func TestNormalizeIP_IPv6(t *testing.T) {
	// /48 means keep first 6 bytes, zero rest.
	got := normalizeIP("2001:db8:abcd:1234:5678:9abc:def0:1234")
	// First 6 bytes: 20 01 0d b8 ab cd → 2001:db8:abcd::
	expected := "2001:db8:abcd::"
	if got != expected {
		t.Errorf("normalizeIP IPv6: got %q, want %q", got, expected)
	}
}

func TestNormalizeIP_Invalid(t *testing.T) {
	// Invalid IP should be returned as-is.
	if got := normalizeIP("not-an-ip"); got != "not-an-ip" {
		t.Errorf("normalizeIP(invalid) = %q, want %q", got, "not-an-ip")
	}
}

// ---------------------------------------------------------------------------
// IsValidSessionID
// ---------------------------------------------------------------------------

func TestIsValidSessionID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"", false},
		{"abcdef1234567890abcdef1234567890", true},     // 32 hex
		{"ABCDEF1234567890ABCDEF1234567890", true},     // uppercase hex
		{"abcdef1234567890abcdef123456789g", false},    // non-hex char
		{"abcdef1234567890abcdef12345678", false},      // 30 chars
		{"  abcdef1234567890abcdef1234567890  ", true}, // trimmed
		{"abcdef1234567890abcdef1234567890ab", false},  // 34 chars
	}

	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			if got := IsValidSessionID(tc.id); got != tc.want {
				t.Errorf("IsValidSessionID(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}
