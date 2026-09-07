package repository

import "strings"

// normalizeRawName lowercases and strips common separators so raw event names
// can be matched against the built-in alias table regardless of styling.
func normalizeRawName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '-', '_', ' ', '.', '/', ':':
			// skip separators
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// builtinAliases maps a normalized raw event name to a canonical key. It only
// covers common, high-confidence cases; anything else yields no suggestion and
// is left for the user to map manually.
var builtinAliases = map[string]string{
	// page_view
	"pageview":   "page_view",
	"pageviewed": "page_view",
	"pageviews":  "page_view",
	"view":       "page_view",
	"screenview": "page_view",
	"visit":      "page_view",
	// sign_up
	"signup":        "sign_up",
	"register":      "sign_up",
	"registration":  "sign_up",
	"registered":    "sign_up",
	"createaccount": "sign_up",
	"accountcreate": "sign_up",
	"joined":        "sign_up",
	// login
	"login":        "login",
	"signin":       "login",
	"loggedin":     "login",
	"loginsuccess": "login",
	// add_to_cart
	"addtocart": "add_to_cart",
	"cartadd":   "add_to_cart",
	"addcart":   "add_to_cart",
	// checkout_start
	"checkout":      "checkout_start",
	"checkoutstart": "checkout_start",
	"begincheckout": "checkout_start",
	"startcheckout": "checkout_start",
	// purchase
	"purchase":         "purchase",
	"purchased":        "purchase",
	"order":            "purchase",
	"ordercompleted":   "purchase",
	"orderplaced":      "purchase",
	"checkoutcomplete": "purchase",
	"payment":          "purchase",
	"paymentsuccess":   "purchase",
}

// stripNamespace removes a leading dotted namespace segment ("seo.page_view" ->
// "page_view"). Returns "" when there is nothing to strip.
//
// Dotted names in production are namespaced, not differently-spelled: the
// platform emits page_view, page.view and seo.page_view for one concept. The
// first two already normalise to the same thing, but a namespace survives
// separator stripping and turns "seo.page_view" into "seopageview", which
// matches nothing — so the names most in need of a suggestion were the ones
// least likely to get one.
//
// Only the FIRST segment is dropped, and only when something follows it. The
// remainder is then matched exactly as any other raw name would be, so a
// namespace cannot manufacture a match that the bare name would not have.
func stripNamespace(raw string) string {
	i := strings.Index(raw, ".")
	if i <= 0 || i == len(raw)-1 {
		return ""
	}
	return raw[i+1:]
}

// guessCanonicalKey returns a best-guess canonical key for a raw event name, or
// "" when there is no confident match. A raw name that already equals a catalog
// key maps to itself; otherwise the built-in alias table is consulted, then the
// same two checks are retried against the name with its namespace stripped. The
// result is only returned when that key exists in the catalog.
//
// Every rule here is syntactic — a different spelling of the same word. Names
// that differ in MEANING are deliberately left unsuggested even when they look
// related: "login_started" is not "login" and "signup_started" is not
// "signup_completed", and collapsing a start into a completion would silently
// inflate every funnel built on it. Those are for a human to map on the event
// mapping page.
func guessCanonicalKey(raw string, catalogKeys map[string]bool) string {
	if key := matchCanonicalKey(raw, catalogKeys); key != "" {
		return key
	}
	if bare := stripNamespace(raw); bare != "" {
		return matchCanonicalKey(bare, catalogKeys)
	}
	return ""
}

// matchCanonicalKey resolves one raw name against the catalog and the built-in
// alias table, comparing on the separator-stripped form.
func matchCanonicalKey(raw string, catalogKeys map[string]bool) string {
	n := normalizeRawName(raw)
	if n == "" {
		return ""
	}
	// Direct hit against a catalog key (compare on normalized form).
	for key := range catalogKeys {
		if normalizeRawName(key) == n {
			return key
		}
	}
	if key, ok := builtinAliases[n]; ok && catalogKeys[key] {
		return key
	}
	return ""
}
