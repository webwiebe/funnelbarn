package api

import (
	"net/http"
	"strings"

	"github.com/wiebe-xyz/funnelbarn/internal/auth"
)

// publicIngestCORSPaths are the endpoints a browser SDK on an arbitrary
// customer origin legitimately calls cross-origin. They authenticate with an
// API-key header (never cookies), so they get open, NON-credentialed CORS.
//
// AUDIT GUARD: do NOT "harden" these by pinning Access-Control-Allow-Headers to
// the fixed corsAllowHeaders list — they must keep reflecting the preflight's
// requested headers (see setCORSHeaders). SDKs/tracing add headers we don't
// control (e.g. `traceparent`); a fixed list drops the missing one and the
// browser blocks the whole request. This regressed once (CORS-1 in #195, fixed
// in #198) and only shows up in a real browser, never in curl or unit tests.
// The fixed allowlist is safe ONLY on the credentialed dashboard API.
var publicIngestCORSPaths = map[string]bool{
	"/api/v1/events":           true,
	"/api/v1/recordings/chunk": true,
	"/api/v1/evaluate":         true,
	"/api/v1/recording-config": true,
	"/api/v1/client-config":    true,
}

// corsAllowHeaders is the fixed set of request headers permitted cross-origin
// on the CREDENTIALED dashboard API only. We never reflect
// Access-Control-Request-Headers verbatim there. NOTE: this list is deliberately
// NOT used for the public ingest paths — see publicIngestCORSPaths.
var corsAllowHeaders = "Content-Type, Authorization, " + auth.HeaderAPIKey +
	", x-funnelbarn-project, x-funnelbarn-environment, X-FunnelBarn-CSRF, X-Requested-With"

func (s *Server) setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}

	// Public ingest endpoints: any origin, but NO credentials (the SDK uses an
	// API-key header, not cookies). Safe to allow broadly precisely because no
	// credentials are attached, so a malicious origin gains nothing.
	if publicIngestCORSPaths[r.URL.Path] {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		// Reflect whatever headers the browser's preflight asks for. These
		// endpoints are NON-credentialed (API-key header, never cookies), so
		// echoing arbitrary request headers is harmless — an attacker origin
		// gains nothing without credentials, and Allow-Headers only governs
		// which request headers the browser may send, it leaks nothing back.
		// A fixed allowlist here is a footgun: SDKs and browser tracing
		// instrumentation add headers we don't control (e.g. `traceparent`),
		// and a missing entry fails the whole preflight. We keep the strict
		// fixed allowlist only for the credentialed dashboard API below.
		if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
			w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
			w.Header().Add("Vary", "Access-Control-Request-Headers")
		} else {
			w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
		}
		w.Header().Set("Access-Control-Max-Age", "86400")
		// Deliberately NO Access-Control-Allow-Credentials here.
		return
	}

	// Everything else is the credentialed dashboard API. Only origins on the
	// explicit allowlist may make credentialed cross-origin requests. An empty
	// allowlist means "same-origin only" (the default deployment serves the SPA
	// from the same origin), NOT "allow everyone" — never reflect an arbitrary
	// origin together with Allow-Credentials.
	allowed := false
	for _, o := range s.allowedOrigins {
		if o == "" {
			continue
		}
		if o == "*" || strings.EqualFold(o, origin) {
			allowed = true
			break
		}
	}
	if !allowed {
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Add("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Max-Age", "86400")
}
