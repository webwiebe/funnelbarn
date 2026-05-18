package auth

import "testing"

func TestOIDCClientAllowed(t *testing.T) {
	cli := NewOIDCClient(OIDCConfig{
		Issuer:        "https://iam.example.com",
		ClientID:      "funnelbarn",
		ClientSecret:  "sek",
		RedirectURL:   "https://funnelbarn.example.com/api/v1/oidc/callback",
		RequiredGroup: "funnelbarn-users",
	})

	cases := []struct {
		name   string
		claims OIDCClaims
		want   bool
	}{
		{"in required group", OIDCClaims{Groups: []string{"funnelbarn-users"}}, true},
		{"owner bypass", OIDCClaims{Roles: []string{"owner"}}, true},
		{"organization_admin bypass", OIDCClaims{Roles: []string{"organization_admin"}}, true},
		{"operator bypass", OIDCClaims{Roles: []string{"operator"}}, true},
		{"other group only", OIDCClaims{Groups: []string{"engineering"}}, false},
		{"no claims", OIDCClaims{}, false},
		{"unrelated role", OIDCClaims{Roles: []string{"member"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cli.Allowed(tc.claims); got != tc.want {
				t.Errorf("Allowed(%+v) = %v, want %v", tc.claims, got, tc.want)
			}
		})
	}
}

func TestOIDCConfigEnabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  OIDCConfig
		want bool
	}{
		{"all set", OIDCConfig{Issuer: "i", ClientID: "c", ClientSecret: "s", RedirectURL: "r"}, true},
		{"missing issuer", OIDCConfig{ClientID: "c", ClientSecret: "s", RedirectURL: "r"}, false},
		{"missing client_id", OIDCConfig{Issuer: "i", ClientSecret: "s", RedirectURL: "r"}, false},
		{"missing secret", OIDCConfig{Issuer: "i", ClientID: "c", RedirectURL: "r"}, false},
		{"missing redirect", OIDCConfig{Issuer: "i", ClientID: "c", ClientSecret: "s"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Enabled(); got != tc.want {
				t.Errorf("Enabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOIDCClaimsPreferredName(t *testing.T) {
	cases := []struct {
		claims OIDCClaims
		want   string
	}{
		{OIDCClaims{PreferredUsername: "alice", Email: "a@x"}, "alice"},
		{OIDCClaims{Email: "a@x"}, "a@x"},
		{OIDCClaims{Name: "Alice"}, "Alice"},
		{OIDCClaims{Subject: "sub-1"}, "sub-1"},
		{OIDCClaims{}, ""},
	}
	for _, tc := range cases {
		if got := tc.claims.PreferredName(); got != tc.want {
			t.Errorf("PreferredName(%+v) = %q, want %q", tc.claims, got, tc.want)
		}
	}
}
