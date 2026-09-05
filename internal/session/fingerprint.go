package session

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"strconv"
	"strings"
	"time"
)

// FingerprintWindow bounds how long one fallback session ID stays in use.
//
// The fingerprint is stateless, so it cannot watch for idle time the way a
// client-side session cookie does. Instead it changes at fixed boundaries: a
// visitor whose activity straddles one is counted as two sessions. That is the
// price of never minting a session that runs for months — before this bound
// existed, a single fingerprint in production collected 39,893 events from 742
// visitors over three months, because nothing in its inputs ever changed.
const FingerprintWindow = 30 * time.Minute

// FingerprintInput carries everything a fallback session ID is derived from.
type FingerprintInput struct {
	// ClientIP is the visitor's own address, as "ip" or "ip:port". It must be
	// the address the request came from originally, not that of a reverse proxy
	// or CDN edge: every visitor behind one ingress shares the proxy's address,
	// so keying on it files all of them under a single session.
	ClientIP string
	// UserAgent may be empty; on its own it separates very little.
	UserAgent string
	// ProjectSlug scopes the session to one project. sessions.id is a global
	// primary key, so two projects behind the same ingress must not collide.
	ProjectSlug string
	// Environment keeps sessions from bleeding across dev, staging, and
	// production when one API key is shared between them.
	Environment string
	// At is when the event occurred. It selects the time window.
	At time.Time
}

// Fingerprint derives an anonymous session ID for an event whose client sent no
// usable session ID of its own. No cookie is required.
//
// The IP is stripped to /24 (IPv4) or /48 (IPv6) before hashing, for a degree
// of k-anonymity while staying stable within a session. The result is
// SHA256(ip | ua | project | env | window) truncated to 32 hex characters.
func Fingerprint(in FingerprintInput) string {
	ip := normalizeIP(extractIP(in.ClientIP))
	window := in.At.UTC().Unix() / int64(FingerprintWindow/time.Second)

	h := sha256.Sum256([]byte(strings.Join([]string{
		ip,
		in.UserAgent,
		in.ProjectSlug,
		in.Environment,
		strconv.FormatInt(window, 10),
	}, "|")))
	return hex.EncodeToString(h[:16]) // 32 hex chars
}

// extractIP strips the port component from a host:port string.
func extractIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

// normalizeIP reduces IP precision to protect privacy.
// IPv4: keep /24 (zero last octet).
// IPv6: keep /48 (zero last 10 bytes).
func normalizeIP(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	}

	if v4 := parsed.To4(); v4 != nil {
		// Zero last octet.
		v4[3] = 0
		return v4.String()
	}

	// IPv6: zero last 10 bytes (keep first 6 bytes = /48).
	v6 := parsed.To16()
	for i := 6; i < 16; i++ {
		v6[i] = 0
	}
	return v6.String()
}

// IsValidSessionID reports whether id has the shape this server mints and the
// SDKs generate: 32 hexadecimal characters. Anything else is replaced by a
// Fingerprint rather than stored as-is.
func IsValidSessionID(id string) bool {
	id = strings.TrimSpace(id)
	return len(id) == 32 && isHex(id)
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
