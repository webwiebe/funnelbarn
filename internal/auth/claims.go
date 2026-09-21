package auth

import "strings"

// OIDCClaims is the subset of ID-token claims this barn cares about.
type OIDCClaims struct {
	Subject           string   `json:"sub"`
	SessionID         string   `json:"sid"` // IdP session id; keys back-channel logout
	Email             string   `json:"email"`
	PreferredUsername string   `json:"preferred_username"`
	Name              string   `json:"name"`
	Groups            []string `json:"groups"`
	Roles             []string `json:"roles"`
}

// PreferredName returns the best human-readable identifier from the claims.
func (c OIDCClaims) PreferredName() string {
	for _, v := range []string{c.PreferredUsername, c.Email, c.Name, c.Subject} {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
