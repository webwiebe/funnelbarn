package api

import "net/url"

// originOf returns the scheme://host origin of a URL, or "" if it can't be
// parsed. Used to whitelist the IAMBarn issuer origin in the CSP.
func originOf(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
