package enrich

import (
	"net/url"
	"regexp"
	"strings"
)

// MaskedSegment replaces a path segment that may carry an identifier or a
// secret in a URLShape.
const MaskedSegment = ":id"

// URLShape names where a sensitive URL parameter appeared without its value:
// the host, the path with identifier-like segments masked, and the parameter
// name. It never holds a parameter value.
type URLShape struct {
	Host  string `json:"host"`
	Path  string `json:"path"`
	Param string `json:"param"`
}

var (
	hexSegment    = regexp.MustCompile(`^[0-9a-fA-F-]{6,}$`)
	base64Segment = regexp.MustCompile(`^[A-Za-z0-9_+=-]{10,}$`)
	digitsOnly    = regexp.MustCompile(`^[0-9]+$`)
	hasDigit      = regexp.MustCompile(`[0-9]`)
	hasLetter     = regexp.MustCompile(`[A-Za-z]`)
)

// maskSegment returns seg, or MaskedSegment when seg is longer than 16
// characters, all digits, hex-shaped or base64-shaped. A word such as
// "magic-links" or "confirm" stays.
func maskSegment(seg string) string {
	switch {
	case len(seg) > 16, digitsOnly.MatchString(seg):
		return MaskedSegment
	case hexSegment.MatchString(seg) && hasDigit.MatchString(seg):
		return MaskedSegment
	case base64Segment.MatchString(seg) && hasDigit.MatchString(seg) && hasLetter.MatchString(seg):
		return MaskedSegment
	}
	return seg
}

// MaskPath masks every segment of a URL path with maskSegment.
func MaskPath(path string) string {
	segs := strings.Split(path, "/")
	for i, s := range segs {
		if s != "" {
			segs[i] = maskSegment(s)
		}
	}
	return strings.Join(segs, "/")
}

// SensitiveURLShapes returns one URLShape per sensitive parameter that carries
// a value in raw, in its query string or in a fragment with parameters, the
// places RedactURL redacts. A hash-router fragment (#/reset?token=...) adds its
// masked route to the path after a "#". The host drops any user info. Nothing
// from a parameter value is returned.
func SensitiveURLShapes(raw string) []URLShape {
	if !looksLikeURL(raw) {
		return nil
	}
	rest, fragment, hasFragment := strings.Cut(raw, "#")
	base, query, _ := strings.Cut(rest, "?")
	host, path := splitHost(base)
	shape := URLShape{Host: host, Path: MaskPath(path)}

	names := sensitiveNames(query)
	if hasFragment {
		route, params, isRouter := strings.Cut(fragment, "?")
		switch {
		case isRouter:
			if strings.HasPrefix(route, "/") {
				shape.Path += "#" + MaskPath(route)
			}
			names = append(names, sensitiveNames(params)...)
		case strings.Contains(fragment, "="):
			names = append(names, sensitiveNames(fragment)...)
		}
	}
	out := make([]URLShape, 0, len(names))
	for _, n := range names {
		s := shape
		s.Param = n
		out = append(out, s)
	}
	return out
}

// splitHost splits an absolute or scheme-relative URL into its lowercased host
// (without user info) and its path. A path-only URL has the host "(relative)".
func splitHost(base string) (host, path string) {
	after, ok := strings.CutPrefix(base, "//")
	if !ok {
		_, rest, hasScheme := strings.Cut(base, "://")
		if !hasScheme {
			return "(relative)", base
		}
		after = rest
	}
	host, path, _ = strings.Cut(after, "/")
	if _, h, hasUser := strings.Cut(host, "@"); hasUser {
		host = h
	}
	return strings.ToLower(host), "/" + path
}

// sensitiveNames returns the names of the sensitive parameters in one
// "k=v&k=v" list that carry a value, lowercased, in order.
func sensitiveNames(params string) []string {
	var names []string
	for _, pair := range strings.Split(params, "&") {
		key, _, hasValue := strings.Cut(pair, "=")
		if !hasValue {
			continue
		}
		name := key
		if decoded, err := url.QueryUnescape(key); err == nil {
			name = decoded
		}
		if IsSensitiveParam(name) {
			names = append(names, strings.ToLower(strings.TrimSpace(name)))
		}
	}
	return names
}
