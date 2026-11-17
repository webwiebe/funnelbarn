package enrich

import "strings"

// RecordingPageURL reduces the page URL stored with a recording to its origin
// and path plus any utm_* parameters. The query and the fragment carry OAuth
// state, nonces, redirect targets and one-time values that a recording list
// has no use for (iambarn#489). The input may be absolute or relative and is
// handled as text, so a URL that net/url rejects is still reduced.
func RecordingPageURL(raw string) string {
	if !strings.ContainsAny(raw, "?#") {
		return raw
	}
	rest, _, _ := strings.Cut(raw, "#")
	base, query, hasQuery := strings.Cut(rest, "?")
	if !hasQuery {
		return base
	}
	var kept []string
	for _, pair := range strings.Split(query, "&") {
		name, _, _ := strings.Cut(pair, "=")
		if strings.HasPrefix(strings.ToLower(name), "utm_") {
			kept = append(kept, pair)
		}
	}
	if len(kept) == 0 {
		return base
	}
	return base + "?" + strings.Join(kept, "&")
}
