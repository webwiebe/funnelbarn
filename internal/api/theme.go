package api

import "net/http"

// themeManifest mirrors the iambarn relying-party theme manifest schema.
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
}

// funnelbarnThemeManifest reflects the FunnelBarn brand as defined in
// web/src/index.css and web/src/components/shell/Shell.tsx (bg, surface,
// amber, text) and web/public/icons.
var funnelbarnThemeManifest = themeManifest{
	Name:            "FunnelBarn",
	LogoURL:         "https://funnelbarn.wiebe.xyz/icons/icon-512.png",
	PrimaryColor:    "#f59e0b",
	BackgroundColor: "#0f1117",
	CardColor:       "#1a1d27",
	BodyTextColor:   "#e2e8f0",
	SupportURL:      "https://funnelbarn.wiebe.xyz/",
	Locale:          "en",
}

// handleThemeManifest serves the iambarn relying-party theme manifest used by
// iambarn to skin its login page when a user is redirected here for OIDC.
func (s *Server) handleThemeManifest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, funnelbarnThemeManifest)
}
