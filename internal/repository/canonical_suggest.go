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
	"pageview":     "page_view",
	"pageviewed":   "page_view",
	"pageviews":    "page_view",
	"view":         "page_view",
	"screenview":   "page_view",
	"visit":        "page_view",
	// sign_up
	"signup":        "sign_up",
	"register":      "sign_up",
	"registration":  "sign_up",
	"registered":    "sign_up",
	"createaccount": "sign_up",
	"accountcreate": "sign_up",
	"joined":        "sign_up",
	// login
	"login":    "login",
	"signin":   "login",
	"loggedin": "login",
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
	"purchase":       "purchase",
	"purchased":      "purchase",
	"order":          "purchase",
	"ordercompleted": "purchase",
	"orderplaced":    "purchase",
	"checkoutcomplete": "purchase",
	"payment":        "purchase",
	"paymentsuccess": "purchase",
}

// guessCanonicalKey returns a best-guess canonical key for a raw event name, or
// "" when there is no confident match. A raw name that already equals a catalog
// key maps to itself; otherwise the built-in alias table is consulted and the
// result is only returned when that key exists in the catalog.
func guessCanonicalKey(raw string, catalogKeys map[string]bool) string {
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
