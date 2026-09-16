package api

import "net/http"

// themeManifest mirrors the iambarn relying-party theme manifest schema
// (iambarn/internal/theme/fetch.go, type Manifest). Keep every field of that
// type here: a field left out is a setting iambarn silently defaults.
// See: https://iam.wiebe.xyz/.well-known/iambarn-theme.json
type themeManifest struct {
	Name            string `json:"name"`
	LogoURL         string `json:"logo_url"`
	PrimaryColor    string `json:"primary_color"`
	BackgroundColor string `json:"background_color"`
	CardColor       string `json:"card_color"`
	BodyTextColor   string `json:"body_text_color"`
	SupportURL      string `json:"support_url"`
	Locale          string `json:"locale"`
	// DefaultLocale is the transactional-mail locale when the user's
	// Accept-Language has no overlap with SupportedLocales.
	DefaultLocale    string   `json:"default_locale,omitempty"`
	SupportedLocales []string `json:"supported_locales,omitempty"`
	// FromAddress is the From: mailbox for auth mail sent on FunnelBarn's
	// behalf. iambarn only accepts it when it shares a registrable domain
	// with the requesting origin, and otherwise falls back to its global
	// sender while keeping FromName.
	FromAddress string            `json:"from_address,omitempty"`
	FromName    string            `json:"from_name,omitempty"`
	Dark        *themeDarkPalette `json:"dark,omitempty"`
}

// themeDarkPalette holds the colours iambarn applies under
// prefers-color-scheme: dark.
type themeDarkPalette struct {
	PrimaryColor    string `json:"primary_color"`
	BackgroundColor string `json:"background_color"`
	CardColor       string `json:"card_color"`
	BodyTextColor   string `json:"body_text_color"`
}

// funnelbarnThemeManifest reflects the FunnelBarn brand as defined in
// web/src/index.css and web/src/components/shell/Shell.tsx (bg, surface,
// amber, text) and web/public/icons. The dashboard is dark-only, so the dark
// palette repeats the top-level colours.
//
// FromAddress is noreply@iam.wiebe.xyz because that is the sender the
// mailserver is configured to sign for; a new funnelbarn.wiebe.xyz mailbox
// would need its own DKIM/SPF setup before mail from it is delivered.
var funnelbarnThemeManifest = themeManifest{
	Name:             "FunnelBarn",
	LogoURL:          "https://funnelbarn.wiebe.xyz/icons/icon-512.png",
	PrimaryColor:     "#f59e0b",
	BackgroundColor:  "#0f1117",
	CardColor:        "#1a1d27",
	BodyTextColor:    "#e2e8f0",
	SupportURL:       "https://funnelbarn.wiebe.xyz/",
	Locale:           "en",
	DefaultLocale:    "en",
	SupportedLocales: []string{"en"},
	FromAddress:      "noreply@iam.wiebe.xyz",
	FromName:         "FunnelBarn",
	Dark: &themeDarkPalette{
		PrimaryColor:    "#f59e0b",
		BackgroundColor: "#0f1117",
		CardColor:       "#1a1d27",
		BodyTextColor:   "#e2e8f0",
	},
}

// handleThemeManifest serves the iambarn relying-party theme manifest used by
// iambarn to skin its login page when a user is redirected here for OIDC.
func (s *Server) handleThemeManifest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, funnelbarnThemeManifest)
}
