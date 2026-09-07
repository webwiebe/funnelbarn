package repository

import "testing"

func catalog() map[string]bool {
	// The six keys 00027 seeds.
	return map[string]bool{
		"page_view": true, "sign_up": true, "login": true,
		"add_to_cart": true, "checkout_start": true, "purchase": true,
	}
}

// The production drift this exists to reconcile: page_view (59,416 events),
// page.view (2,259) and seo.page_view (626) are one concept. Separator
// stripping already handled the first two; a namespace survived it and turned
// "seo.page_view" into "seopageview", which matched nothing — so the names most
// in need of a suggestion were the ones least likely to get one.
func TestGuessCanonicalKey_NamespacedNames(t *testing.T) {
	keys := catalog()
	for _, tc := range []struct{ raw, want string }{
		{"page_view", "page_view"},
		{"page.view", "page_view"},
		{"PAGE-VIEW", "page_view"},
		{"seo.page_view", "page_view"},
		{"app.sign_up", "sign_up"},
		{"web.checkout", "checkout_start"},
		{"shop.purchased", "purchase"},
	} {
		if got := guessCanonicalKey(tc.raw, keys); got != tc.want {
			t.Errorf("guessCanonicalKey(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// Only names that are the same WORD are matched. A name that differs in meaning
// gets no suggestion even when it looks related: collapsing a start into a
// completion would silently inflate every funnel built on it.
func TestGuessCanonicalKey_LeavesSemanticDifferencesAlone(t *testing.T) {
	keys := catalog()
	for _, raw := range []string{
		"login_started",  // a login attempt is not a login
		"auth.initiated", // same, namespaced
		"signup_started", // not the same event as signup_completed
		"qr.created",     // "created" means nothing on its own
		"invite_created",
		"page_engaged",
		"sketch.queued",
	} {
		if got := guessCanonicalKey(raw, keys); got != "" {
			t.Errorf("guessCanonicalKey(%q) = %q, want no suggestion", raw, got)
		}
	}
}

// A namespace must not manufacture a match the bare name would not have had,
// and only the first segment is ever dropped.
func TestStripNamespace(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"seo.page_view", "page_view"},
		{"a.b.c", "b.c"}, // one segment only
		{"page_view", ""},
		{".leading", ""},  // nothing before the dot
		{"trailing.", ""}, // nothing after it
		{"", ""},
		{".", ""},
	} {
		if got := stripNamespace(tc.raw); got != tc.want {
			t.Errorf("stripNamespace(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// A key must exist in the catalog before it is suggested — a mapping to a key
// that is not there would violate the foreign key on insert.
func TestGuessCanonicalKey_RespectsTheCatalog(t *testing.T) {
	only := map[string]bool{"login": true}
	if got := guessCanonicalKey("seo.page_view", only); got != "" {
		t.Errorf("suggested %q for a key not in the catalog", got)
	}
	if got := guessCanonicalKey("app.signin", only); got != "login" {
		t.Errorf("guessCanonicalKey(app.signin) = %q, want login", got)
	}
}

func TestNormalizeRawName(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"page_view", "pageview"},
		{"  Page-View  ", "pageview"},
		{"seo/page:view", "seopageview"},
		{"", ""},
	} {
		if got := normalizeRawName(tc.raw); got != tc.want {
			t.Errorf("normalizeRawName(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}
