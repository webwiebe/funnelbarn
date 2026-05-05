package session

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Fingerprint
// ---------------------------------------------------------------------------

func TestFingerprint_ReturnsHex32(t *testing.T) {
	fp := Fingerprint("192.168.1.42:54321", "Mozilla/5.0 Chrome/124")
	if len(fp) != 32 {
		t.Errorf("expected 32 hex chars, got %d: %q", len(fp), fp)
	}
	if !isHex(fp) {
		t.Errorf("fingerprint is not hex: %q", fp)
	}
}

func TestFingerprint_Deterministic(t *testing.T) {
	a := Fingerprint("10.0.0.5:8080", "some-ua")
	b := Fingerprint("10.0.0.5:8080", "some-ua")
	if a != b {
		t.Errorf("Fingerprint not deterministic: %q != %q", a, b)
	}
}

func TestFingerprint_DifferentInputsDifferentOutput(t *testing.T) {
	a := Fingerprint("10.0.0.5:8080", "ua-1")
	b := Fingerprint("10.0.0.5:8080", "ua-2")
	if a == b {
		t.Error("different UAs should produce different fingerprints")
	}
}

// ---------------------------------------------------------------------------
// IPv4 /24 anonymization
// ---------------------------------------------------------------------------

func TestFingerprint_IPv4AnonymizationSlash24(t *testing.T) {
	// 192.168.1.5 and 192.168.1.99 share /24 prefix → same fingerprint.
	fpA := Fingerprint("192.168.1.5:1000", "ua")
	fpB := Fingerprint("192.168.1.99:2000", "ua")
	if fpA != fpB {
		t.Errorf("IPv4 /24: same subnet should fingerprint identically, got %q vs %q", fpA, fpB)
	}

	// Different /24 subnet → different fingerprint.
	fpC := Fingerprint("192.168.2.5:1000", "ua")
	if fpA == fpC {
		t.Errorf("IPv4: different /24 subnets should produce different fingerprints")
	}
}

func TestFingerprint_IPv4WithoutPort(t *testing.T) {
	// When remoteAddr has no port, it should still work.
	fp := Fingerprint("10.0.0.1", "ua")
	if len(fp) != 32 {
		t.Errorf("expected 32 chars, got %d: %q", len(fp), fp)
	}
}

func TestFingerprint_IPv4LastOctetIgnored(t *testing.T) {
	// Simpler: check a few specific IPs in 172.16.0.x — all map to same /24.
	base := Fingerprint("172.16.0.1:9000", "ua")
	for _, suffix := range []string{"2", "100", "200", "255"} {
		fp := Fingerprint("172.16.0."+suffix+":9000", "ua")
		if fp != base {
			t.Errorf("172.16.0.%s should match 172.16.0.1 (same /24), got different fingerprint", suffix)
		}
	}
}

// ---------------------------------------------------------------------------
// IPv6 /48 anonymization
// ---------------------------------------------------------------------------

func TestFingerprint_IPv6AnonymizationSlash48(t *testing.T) {
	// 2001:db8:1234::1 and 2001:db8:1234::2 share /48.
	fpA := Fingerprint("[2001:db8:1234::1]:80", "ua")
	fpB := Fingerprint("[2001:db8:1234::2]:80", "ua")
	if fpA != fpB {
		t.Errorf("IPv6 /48: same prefix should fingerprint identically, got %q vs %q", fpA, fpB)
	}

	// Different /48 prefix → different fingerprint.
	fpC := Fingerprint("[2001:db8:5678::1]:80", "ua")
	if fpA == fpC {
		t.Errorf("IPv6: different /48 prefixes should produce different fingerprints")
	}
}

func TestFingerprint_IPv6NoBrackets(t *testing.T) {
	// Without brackets+port (raw IPv6 addr as remoteAddr).
	fp := Fingerprint("::1", "ua")
	if len(fp) != 32 {
		t.Errorf("expected 32 chars, got %d: %q", len(fp), fp)
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
		got := normalizeIP(tc.ip)
		if got != tc.want {
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
	got := normalizeIP("not-an-ip")
	if got != "not-an-ip" {
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
			got := IsValidSessionID(tc.id)
			if got != tc.want {
				t.Errorf("IsValidSessionID(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Fingerprint produces a valid session ID
// ---------------------------------------------------------------------------

func TestFingerprint_IsValidSessionID(t *testing.T) {
	fp := Fingerprint("203.0.113.10:12345", "Mozilla/5.0")
	if !IsValidSessionID(fp) {
		t.Errorf("Fingerprint result %q should be a valid session ID", fp)
	}
}
