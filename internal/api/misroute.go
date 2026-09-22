package api

import (
	"log/slog"
	"net/http"
	"strings"
)

// redirectVanityHost handles the bare-host redirect for the f.<domain> vanity
// ingest hosts: a browser navigating to https://f.example.com/ (root, or any
// non-ingest path) gets 301'd to https://example.com — strip the "f." label and
// send it to the app the host fronts. e.g. f.profotograaf.nl → profotograaf.nl.
//
// This is the app half of the wildcard vanity-host feature. The edge
// (deploy/k8s/.../ingressroute-f-wildcard.yaml) routes the ingest API and
// SDK bundle on any f.<domain> to their services and sends every other path
// here, so this redirect works for any customer domain with no per-project
// ingress wiring. Only GET/HEAD navigations redirect (curl -sI sends HEAD);
// the ingest/SDK paths pass through to their handlers untouched. It reports
// whether it wrote the redirect.
func redirectVanityHost(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	host := r.Host
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	if !strings.HasPrefix(host, "f.") || isIngestPassthroughPath(r.URL.Path) {
		return false
	}
	http.Redirect(w, r, "https://"+strings.TrimPrefix(host, "f."), http.StatusMovedPermanently)
	return true
}

// canonicalAPIPath detects a path whose client doubled the API prefix — the
// shape produced by joining a base URL that already ends in the ingest path
// with the path again — and returns the path it meant.
//
//	/api/v1/events/api/v1/events          -> /api/v1/events
//	/api/api/v1/evaluate                  -> /api/v1/evaluate
//	/api/v1/events/api/v1/recording-config -> /api/v1/recording-config
//
// It keys on the LAST occurrence of the prefix, so the result always contains
// exactly one and a redirect can never loop. A path with a single prefix (the
// normal case, and genuinely-wrong paths like /api/v1/recordings/chunk) is not
// rewritten — those still 404, and logMisroutedRequest reports them.
func canonicalAPIPath(path string) (string, bool) {
	const prefix = "/api/v1/"
	i := strings.LastIndex(path, prefix)
	if i <= 0 {
		return "", false
	}
	return path[i:], true
}

// canonicalMetricPath bounds the cardinality of the misrouted-request counter.
// An unmatched path is caller-controlled, so only paths we actually serve are
// used as label values; anything else is bucketed as "other" rather than
// minting a new series per scanner probe.
func canonicalMetricPath(path string) string {
	if _, ok := knownAPIPaths[path]; ok {
		return path
	}
	if canonical, ok := canonicalAPIPath(path); ok {
		if _, known := knownAPIPaths[canonical]; known {
			return canonical
		}
	}
	return "other"
}

// knownAPIPaths is the set of fixed ingest-facing routes worth distinguishing
// in the misroute counter. Parameterised dashboard routes are deliberately
// absent — they are not what misconfigured clients post to, and they would
// need templating to stay bounded.
var knownAPIPaths = map[string]struct{}{
	"/api/v1/events":           {},
	"/api/v1/evaluate":         {},
	"/api/v1/recordings/chunk": {},
	"/api/v1/recording-config": {},
	"/api/v1/setup":            {},
	"/api/v1/health":           {},
}

// logMisroutedRequest reports a request that did not reach a real route, with
// the identifying headers needed to trace it back to a repository. The audit
// that found this could not attribute 189 lost events a week to any client
// because nothing recorded the origin or the project they claimed.
func logMisroutedRequest(r *http.Request, canonical string) {
	attrs := []any{
		"handled", canonical != "",
		"method", r.Method,
		"path", r.URL.Path,
		"origin", r.Header.Get("Origin"),
		"referer", r.Header.Get("Referer"),
		"user_agent", r.Header.Get("User-Agent"),
		"project", r.Header.Get("x-funnelbarn-project"),
	}
	if canonical != "" {
		attrs = append(attrs, "redirected_to", canonical)
		slog.WarnContext(r.Context(), "request used a doubled API path; redirecting to the canonical one", attrs...)
		return
	}
	slog.WarnContext(r.Context(), "request did not match any route", attrs...)
}
