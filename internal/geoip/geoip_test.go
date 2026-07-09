package geoip

import "testing"

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
