package enrich

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
)

// UAInfo contains parsed user-agent details.
type UAInfo struct {
	Browser    string
	OS         string
	DeviceType string // "desktop", "mobile", "tablet", "bot"
}

// ParseUA parses a User-Agent string into structured fields.
// This is a lightweight heuristic parser without a full UA database.
func ParseUA(ua string) UAInfo {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return UAInfo{DeviceType: "unknown"}
	}

	lower := strings.ToLower(ua)

	// Detect bots first.
	if isBot(lower) {
		return UAInfo{Browser: "Bot", OS: "Bot", DeviceType: "bot"}
	}

	deviceType := "desktop"
	if strings.Contains(lower, "mobile") || strings.Contains(lower, "android") {
		if strings.Contains(lower, "tablet") || strings.Contains(lower, "ipad") {
			deviceType = "tablet"
		} else {
			deviceType = "mobile"
		}
	} else if strings.Contains(lower, "tablet") || strings.Contains(lower, "ipad") {
		deviceType = "tablet"
	}

	return UAInfo{
		Browser:    parseBrowser(lower),
		OS:         parseOS(lower),
		DeviceType: deviceType,
	}
}

// browserRules are tried in order: most browsers also name the ones they derive
// from, so Edge has to match before Chrome and Chrome before Safari.
var browserRules = []struct {
	name    string
	anyOf   []string
	without string
}{
	{name: "Edge", anyOf: []string{"edg/", "edge/"}},
	{name: "Opera", anyOf: []string{"opr/", "opera"}},
	{name: "Chrome", anyOf: []string{"chrome"}, without: "chromium"},
	{name: "Chromium", anyOf: []string{"chromium"}},
	{name: "Firefox", anyOf: []string{"firefox", "fxios"}},
	{name: "Safari", anyOf: []string{"safari"}, without: "chrome"},
	{name: "IE", anyOf: []string{"msie", "trident/"}},
	{name: "curl", anyOf: []string{"curl"}},
}

func parseBrowser(lower string) string {
	for _, rule := range browserRules {
		if rule.without != "" && strings.Contains(lower, rule.without) {
			continue
		}
		for _, sub := range rule.anyOf {
			if strings.Contains(lower, sub) {
				return rule.name
			}
		}
	}
	return "Other"
}

func parseOS(lower string) string {
	switch {
	case strings.Contains(lower, "windows"):
		return "Windows"
	case strings.Contains(lower, "mac os") || strings.Contains(lower, "macos") || strings.Contains(lower, "darwin"):
		return "macOS"
	case strings.Contains(lower, "iphone") || strings.Contains(lower, "ipad") || strings.Contains(lower, "ios"):
		return "iOS"
	case strings.Contains(lower, "android"):
		return "Android"
	case strings.Contains(lower, "linux"):
		return "Linux"
	case strings.Contains(lower, "freebsd") || strings.Contains(lower, "openbsd"):
		return "BSD"
	default:
		return "Other"
	}
}

func isBot(lower string) bool {
	bots := []string{
		"bot", "crawler", "spider", "slurp", "googlebot", "bingbot",
		"yandexbot", "duckduckbot", "baiduspider", "facebookexternalhit",
		"lighthouse", "headlesschrome", "phantomjs", "puppeteer",
	}
	for _, b := range bots {
		if strings.Contains(lower, b) {
			return true
		}
	}
	return false
}

// ExtractReferrerDomain parses the domain from a referrer URL.
func ExtractReferrerDomain(referrer string) string {
	if referrer == "" {
		return ""
	}
	u, err := url.Parse(referrer)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	// Strip leading www.
	host = strings.TrimPrefix(host, "www.")
	return host
}

// UTMParams holds extracted UTM attribution parameters.
type UTMParams struct {
	Source   string
	Medium   string
	Campaign string
	Term     string
	Content  string
}

// ExtractUTM extracts UTM parameters from a URL.
func ExtractUTM(rawURL string) UTMParams {
	if rawURL == "" {
		return UTMParams{}
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return UTMParams{}
	}
	q := u.Query()
	return UTMParams{
		Source:   q.Get("utm_source"),
		Medium:   q.Get("utm_medium"),
		Campaign: q.Get("utm_campaign"),
		Term:     q.Get("utm_term"),
		Content:  q.Get("utm_content"),
	}
}

// HashUserID computes a SHA-256 hex digest of the user ID for privacy-safe storage.
// Returns an empty string if userID is empty.
func HashUserID(userID string) string {
	if userID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(userID))
	return hex.EncodeToString(sum[:])
}

// StripURL removes query parameters and fragments from a URL, keeping only scheme + host + path.
func StripURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
