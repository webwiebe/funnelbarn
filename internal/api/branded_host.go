package api

import (
	"net"
	"net/http"
	"strings"
)

// Helpers for the f.<brand> alias hosts that front FunnelBarn ingest
// (deploy/k8s/production/ingressroute-f-wildcard.yaml, deploy/RUNBOOK.md §2b).

// setupBaseURL is the origin the setup guide hands out. A customer onboarding
// through its branded alias (f.acme.nl/api/v1/setup/x) gets that alias back, so
// the snippet they paste keeps their brand; every other host gets the canonical
// FUNNELBARN_PUBLIC_URL. The alias is always https: the f.* edge route only
// listens on websecure (deploy/k8s/production/ingressroute-f-wildcard.yaml).
func (s *Server) setupBaseURL(r *http.Request) string {
	if host := s.requestHost(r); isBrandedAliasHost(host) {
		return "https://" + host
	}
	return s.defaultPublicURL()
}

// defaultPublicURL is the base URL used when there is no branded-alias host
// to prefer: the configured FUNNELBARN_PUBLIC_URL, or the canonical
// funnelbarn.wiebe.xyz. setupBaseURL uses it for a non-branded request; an
// MCP tool call has no *http.Request to read a Host from at all, so it uses
// this directly (see Server.SetupDoc).
func (s *Server) defaultPublicURL() string {
	if s.publicURL != "" {
		return s.publicURL
	}
	return "https://funnelbarn.wiebe.xyz"
}

// requestHost returns the lowercased host the client addressed, without a
// port. X-Forwarded-Host is honoured under the same trust rule as
// X-Forwarded-Proto in isSecureRequest: always when no trusted proxies are
// configured, otherwise only from a configured trusted proxy.
func (s *Server) requestHost(r *http.Request) string {
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" && s.trustsForwardedHeaders(r) {
		host = strings.TrimSpace(strings.SplitN(fwd, ",", 2)[0])
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(host)
}

// isBrandedAliasHost reports whether host is an f.<brand> alias: the reserved
// "f." label followed by a syntactically valid DNS name with at least two
// labels. The strict charset matters because the host is echoed into the
// setup guide, which people paste into their sites.
func isBrandedAliasHost(host string) bool {
	rest, ok := strings.CutPrefix(host, "f.")
	if !ok || len(rest) > 253 || !strings.Contains(rest, ".") {
		return false
	}
	for _, label := range strings.Split(rest, ".") {
		if !isDNSLabel(label) {
			return false
		}
	}
	return true
}

// isDNSLabel reports whether label is a lowercase LDH label (RFC 1123).
func isDNSLabel(label string) bool {
	if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for _, c := range label {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

// isIngestPassthroughPath reports whether a request on an f.<domain> vanity
// host should pass through to its handler instead of being 301-redirected to
// the app root. These mirror the paths the edge IngressRoute allows on wildcard
// f.* hosts: the ingest/config API (/api/*) and the browser SDK bundle, plus the
// iambarn theme manifest. Every other path is a stray browser hit that belongs on the real app domain.
func isIngestPassthroughPath(path string) bool {
	return strings.HasPrefix(path, "/api") ||
		path == "/sdk.js" ||
		path == "/sdk/funnelbarn.js" ||
		path == themeManifestPath
}

// themeManifestPath is fetched by iambarn from whichever origin started the
// login, including a branded host that serves the dashboard
// (f.bananasketch.nl). iambarn treats any 3xx as a failed fetch, so the
// f.<domain> redirect must leave it alone.
const themeManifestPath = "/.well-known/iambarn-theme.json"
