package session

import (
	"crypto/sha256"
	"fmt"
	"net"
	"strings"
)

// Fingerprint generates an anonymous session ID from the remote address and
// user-agent. No cookie is required. The fingerprint is SHA256(ip + "|" + ua)
// truncated to 32 hex characters.
//
// The IP is stripped to /24 (IPv4) or /48 (IPv6) before hashing to provide
// a degree of k-anonymity while still being stable within a session.
func Fingerprint(remoteAddr, userAgent string) string {
	ip := extractIP(remoteAddr)
	normalized := normalizeIP(ip)

	h := sha256.Sum256([]byte(normalized + "|" + userAgent))
	return fmt.Sprintf("%x", h[:16]) // 32 hex chars
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

// SessionExpired returns true when the lastSeen timestamp string indicates
// that a 30-minute idle window has passed compared to now (string comparison
// in RFC3339 format works because the format is lexicographically sortable).
// Callers pass RFC3339 timestamps; this function exists purely as a helper
// for the JS SDK session rotation logic and is not used server-side.
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
