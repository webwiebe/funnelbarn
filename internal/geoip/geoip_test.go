package geoip

import (
	"context"
	"testing"
)

func TestOpen_NonexistentCityDBReturnsError(t *testing.T) {
	l, err := Open("/no/such/geolite2-city.mmdb", "")
	if err == nil {
		t.Fatal("expected error opening a missing city database, got nil")
	}
	if l != nil {
		t.Errorf("expected nil Lookup on error, got %v", l)
	}
}

func TestOpen_NonexistentASNDBReturnsError(t *testing.T) {
	// Even with an unreadable ASN path, Open must fail (the city path is also
	// missing here, so an error is expected without touching a real DB).
	l, err := Open("/no/such/city.mmdb", "/no/such/asn.mmdb")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if l != nil {
		t.Errorf("expected nil Lookup on error, got %v", l)
	}
}

func TestNilLookup_IsSafe(t *testing.T) {
	var l *Lookup
	if got := l.Lookup("1.2.3.4"); got != nil {
		t.Errorf("nil Lookup.Lookup should return nil, got %v", got)
	}
	// Close on a nil receiver must not panic.
	l.Close()
}

func TestClassifyASN(t *testing.T) {
	cases := []struct {
		org  string
		want string
	}{
		{"Amazon.com, Inc.", "datacenter"},
		{"Google LLC", "datacenter"},
		{"Hetzner Online GmbH", "datacenter"},
		{"Some Hosting Provider", "datacenter"},
		{"T-Mobile US", "mobile"},
		{"Vodafone Wireless", "mobile"},
		{"Comcast Cable Communications", "residential"},
		{"", "residential"},
	}
	for _, tc := range cases {
		if got := classifyASN(tc.org); got != tc.want {
			t.Errorf("classifyASN(%q) = %q, want %q", tc.org, got, tc.want)
		}
	}
}

// LookupContext must reject an address it cannot parse before it dereferences
// cityDB. Until this test existed the only Lookup calls were on a nil receiver,
// so every statement past the nil check was untested and the ordering of the
// two guards was load-bearing but unverified.
func TestLookupContext_UnparseableAddressReturnsNilWithoutTouchingDB(t *testing.T) {
	// No databases opened: reaching one would panic, which is the point.
	l := &Lookup{}

	for _, addr := range []string{
		"",
		"not-an-ip",
		"example.com",
		"example.com:443", // host:port splits, but the host is still not an IP
		"1.2.3.4:80:90",   // SplitHostPort fails; the whole string is not an IP
	} {
		t.Run(addr, func(t *testing.T) {
			if got := l.LookupContext(context.Background(), addr); got != nil {
				t.Errorf("LookupContext(%q) = %v, want nil", addr, got)
			}
		})
	}
}

func TestLookup_DelegatesToLookupContext(t *testing.T) {
	l := &Lookup{}
	if got := l.Lookup("not-an-ip"); got != nil {
		t.Errorf("Lookup on an unparseable address = %v, want nil", got)
	}
}
