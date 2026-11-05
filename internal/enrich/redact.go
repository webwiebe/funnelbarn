package enrich

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
)

// RedactedValue replaces the value of a sensitive URL parameter. The key stays,
// so a stored URL still shows which parameters the page carried.
const RedactedValue = "redacted"

// exactSensitiveParams are parameter names whose values are dropped as a whole
// word. code, state and invite are too short to match as substrings: "zipcode"
// or "statement" are ordinary parameters.
var exactSensitiveParams = map[string]bool{
	"token":  true,
	"code":   true,
	"state":  true,
	"invite": true,
}

// sensitiveParamParts are substrings that mark a parameter name sensitive
// wherever they appear in it (access_token, client_secret, new_password).
var sensitiveParamParts = []string{"token", "secret", "password"}

// IsSensitiveParam reports whether a URL parameter name carries a value that
// must not be stored. The comparison ignores case.
func IsSensitiveParam(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	if exactSensitiveParams[n] {
		return true
	}
	for _, part := range sensitiveParamParts {
		if strings.Contains(n, part) {
			return true
		}
	}
	return false
}

// RedactURL replaces the values of sensitive parameters in a URL's query
// string and in a fragment that carries parameters (#access_token=... or a
// hash-router path such as #/reset?token=...). Everything else in the string,
// including parameter order and encoding, stays as it was. The input may be
// absolute or relative; it is handled as text, so a URL that net/url rejects
// is still redacted.
func RedactURL(raw string) string {
	if raw == "" || !strings.ContainsAny(raw, "?#") {
		return raw
	}

	rest, fragment, hasFragment := strings.Cut(raw, "#")
	base, query, hasQuery := strings.Cut(rest, "?")

	out := base
	if hasQuery {
		out += "?" + redactParams(query)
	}
	if hasFragment {
		out += "#" + redactFragment(fragment)
	}
	return out
}

// redactFragment redacts the parameters in a fragment. A fragment with a "?"
// is a hash-router path and its parameters follow the "?". A fragment without
// one is a parameter list itself only when it has an "=" (#access_token=...);
// a plain anchor (#pricing) is left alone.
func redactFragment(fragment string) string {
	if path, params, ok := strings.Cut(fragment, "?"); ok {
		return path + "?" + redactParams(params)
	}
	if strings.Contains(fragment, "=") {
		return redactParams(fragment)
	}
	return fragment
}

// redactParams redacts one "k=v&k=v" list, keeping each pair in place. A
// sensitive key with no "=" keeps its shape: it carries no value to drop.
func redactParams(params string) string {
	if params == "" {
		return params
	}
	pairs := strings.Split(params, "&")
	for i, pair := range pairs {
		key, _, hasValue := strings.Cut(pair, "=")
		if !hasValue {
			continue
		}
		name := key
		if decoded, err := url.QueryUnescape(key); err == nil {
			name = decoded
		}
		if IsSensitiveParam(name) {
			pairs[i] = key + "=" + RedactedValue
		}
	}
	return strings.Join(pairs, "&")
}

// RedactURLStrings walks an event's properties and redacts every string value
// that looks like a URL, at any depth. The map is changed in place and also
// returned. Values that are not URL-shaped are left alone: free text that
// happens to contain "?" is the caller's data.
func RedactURLStrings(props map[string]any) map[string]any {
	for k, v := range props {
		props[k] = redactValue(v)
	}
	return props
}

func redactValue(v any) any {
	switch t := v.(type) {
	case string:
		if looksLikeURL(t) {
			return RedactURL(t)
		}
		return t
	case map[string]any:
		return RedactURLStrings(t)
	case []any:
		for i := range t {
			t[i] = redactValue(t[i])
		}
		return t
	default:
		return v
	}
}

// looksLikeURL reports whether s is an absolute URL, a scheme-relative URL or
// a path, with a query or fragment to redact.
func looksLikeURL(s string) bool {
	if !strings.ContainsAny(s, "?#") || strings.ContainsAny(s, " \n\t") {
		return false
	}
	return strings.Contains(s, "://") || strings.HasPrefix(s, "/")
}

// sensitiveMarkers are cheap byte checks that come before decoding a JSON
// document. A document without any of them has no sensitive URL parameter, so
// the common case skips the decode.
var sensitiveMarkers = [][]byte{
	[]byte("token"), []byte("secret"), []byte("password"),
	[]byte("code="), []byte("state="), []byte("invite="),
}

// RedactURLsInJSON redacts every URL-shaped string value in a JSON document,
// at any depth, with the same rules as RedactURL. It reports whether anything
// changed. An unchanged document is returned as given, byte for byte. A changed
// one is re-encoded with its numbers kept exactly (json.Number) and without
// HTML escaping, so markup inside string values stays as it was.
func RedactURLsInJSON(raw []byte) ([]byte, bool, error) {
	lower := bytes.ToLower(raw)
	marked := false
	for _, m := range sensitiveMarkers {
		if bytes.Contains(lower, m) {
			marked = true
			break
		}
	}
	if !marked {
		return raw, false, nil
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return raw, false, err
	}
	changed := false
	doc = redactJSONValue(doc, &changed)
	if !changed {
		return raw, false, nil
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return raw, false, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), true, nil
}

func redactJSONValue(v any, changed *bool) any {
	switch t := v.(type) {
	case string:
		if !looksLikeURL(t) {
			return t
		}
		r := RedactURL(t)
		if r != t {
			*changed = true
		}
		return r
	case map[string]any:
		for k, child := range t {
			t[k] = redactJSONValue(child, changed)
		}
		return t
	case []any:
		for i := range t {
			t[i] = redactJSONValue(t[i], changed)
		}
		return t
	default:
		return v
	}
}
