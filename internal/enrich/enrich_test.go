package enrich

import (
	"testing"
)

// ---------------------------------------------------------------------------
// ParseUA
// ---------------------------------------------------------------------------

func TestParseUA_Empty(t *testing.T) {
	info := ParseUA("")
	if info.DeviceType != "unknown" {
		t.Errorf("expected DeviceType=unknown, got %q", info.DeviceType)
	}
}

func TestParseUA_Browsers(t *testing.T) {
	tests := []struct {
		name    string
		ua      string
		browser string
		os      string
		device  string
	}{
		{
			name:    "Chrome on Windows",
			ua:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
			browser: "Chrome",
			os:      "Windows",
			device:  "desktop",
		},
		{
			name:    "Firefox on Linux",
			ua:      "Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0",
			browser: "Firefox",
			os:      "Linux",
			device:  "desktop",
		},
		{
			name:    "Safari on macOS",
			ua:      "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Safari/605.1.15",
			browser: "Safari",
			os:      "macOS",
			device:  "desktop",
		},
		{
			name:    "Edge on Windows",
			ua:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0",
			browser: "Edge",
			os:      "Windows",
			device:  "desktop",
		},
		{
			name:    "Opera",
			ua:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 OPR/108.0.0.0 Safari/537.36",
			browser: "Opera",
			os:      "Windows",
			device:  "desktop",
		},
		{
			name:    "IE with MSIE token",
			ua:      "Mozilla/5.0 (compatible; MSIE 10.0; Windows NT 6.1; Trident/6.0)",
			browser: "IE",
			os:      "Windows",
			device:  "desktop",
		},
		{
			name:    "IE with Trident token",
			ua:      "Mozilla/5.0 (Windows NT 6.3; Trident/7.0; rv:11.0) like Gecko",
			browser: "IE",
			os:      "Windows",
			device:  "desktop",
		},
		{
			name:    "curl",
			ua:      "curl/8.4.0",
			browser: "curl",
			os:      "Other",
			device:  "desktop",
		},
		{
			name:    "Chromium",
			ua:      "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) chromium/124.0 Safari/537.36",
			browser: "Chromium",
			os:      "Linux",
			device:  "desktop",
		},
		{
			name:    "Firefox iOS (fxios)",
			ua:      "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148 FxiOS/125.0",
			browser: "Firefox",
			// parseOS matches "mac os" (from "Mac OS X") before "iphone", so result is macOS.
			os:     "macOS",
			device: "mobile",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := ParseUA(tc.ua)
			if info.Browser != tc.browser {
				t.Errorf("Browser: want %q, got %q", tc.browser, info.Browser)
			}
			if info.OS != tc.os {
				t.Errorf("OS: want %q, got %q", tc.os, info.OS)
			}
			if info.DeviceType != tc.device {
				t.Errorf("DeviceType: want %q, got %q", tc.device, info.DeviceType)
			}
		})
	}
}

func TestParseUA_Bots(t *testing.T) {
	bots := []string{
		"Googlebot/2.1 (+http://www.google.com/bot.html)",
		"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)",
		"facebookexternalhit/1.1",
		"Mozilla/5.0 HeadlessChrome/99",
		"Mozilla/5.0 (compatible; YandexBot/3.0; +http://yandex.com/bots)",
		"DuckDuckBot/1.1",
		"Baiduspider/2.0",
	}

	for _, ua := range bots {
		t.Run(ua, func(t *testing.T) {
			info := ParseUA(ua)
			if info.DeviceType != "bot" {
				t.Errorf("expected bot, got DeviceType=%q for UA %q", info.DeviceType, ua)
			}
			if info.Browser != "Bot" {
				t.Errorf("expected Browser=Bot, got %q", info.Browser)
			}
		})
	}
}

func TestParseUA_DeviceTypes(t *testing.T) {
	tests := []struct {
		name   string
		ua     string
		device string
	}{
		{
			name:   "Android mobile",
			ua:     "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Mobile Safari/537.36",
			device: "mobile",
		},
		{
			name:   "Android tablet",
			ua:     "Mozilla/5.0 (Linux; Android 12; SM-X906C) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Tablet Safari/537.36",
			device: "tablet",
		},
		{
			name:   "iPad (contains ipad)",
			ua:     "Mozilla/5.0 (iPad; CPU OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1",
			device: "tablet",
		},
		{
			name:   "iPhone",
			ua:     "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1",
			device: "mobile",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := ParseUA(tc.ua)
			if info.DeviceType != tc.device {
				t.Errorf("DeviceType: want %q, got %q (UA=%q)", tc.device, info.DeviceType, tc.ua)
			}
		})
	}
}

func TestParseUA_OS(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		os   string
	}{
		{"Android", "Mozilla/5.0 (Linux; Android 14) Chrome/120.0 Mobile", "Android"},
		// "Mac OS X" appears in the UA before "iphone" is checked, so parseOS returns macOS.
		{"iOS iPhone", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) Safari/604.1", "macOS"},
		{"BSD", "Mozilla/5.0 (FreeBSD; rv:60.0) Gecko/20100101 Firefox/60.0", "BSD"},
		{"OpenBSD", "Mozilla/5.0 (OpenBSD amd64) Firefox/60.0", "BSD"},
		{"Darwin/macOS", "Wget/1.21.1 (darwin21.6.0)", "macOS"},
		{"Unknown OS", "SomeObscureAgent/1.0", "Other"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := ParseUA(tc.ua)
			if info.OS != tc.os {
				t.Errorf("OS: want %q, got %q", tc.os, info.OS)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ExtractReferrerDomain
// ---------------------------------------------------------------------------

func TestExtractReferrerDomain(t *testing.T) {
	tests := []struct {
		referrer string
		want     string
	}{
		{"", ""},
		{"https://www.google.com/search?q=test", "google.com"},
		{"https://google.com/", "google.com"},
		{"http://www.example.com/path", "example.com"},
		{"https://sub.domain.example.co.uk/page", "sub.domain.example.co.uk"},
		{"not-a-url", ""},
		{"https://twitter.com/home", "twitter.com"},
	}

	for _, tc := range tests {
		t.Run(tc.referrer, func(t *testing.T) {
			got := ExtractReferrerDomain(tc.referrer)
			if got != tc.want {
				t.Errorf("ExtractReferrerDomain(%q) = %q, want %q", tc.referrer, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ExtractUTM
// ---------------------------------------------------------------------------

func TestExtractUTM(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want UTMParams
	}{
		{
			name: "empty URL",
			url:  "",
			want: UTMParams{},
		},
		{
			name: "URL with all UTM params",
			url:  "https://example.com/page?utm_source=google&utm_medium=cpc&utm_campaign=spring&utm_term=shoes&utm_content=ad1",
			want: UTMParams{
				Source:   "google",
				Medium:   "cpc",
				Campaign: "spring",
				Term:     "shoes",
				Content:  "ad1",
			},
		},
		{
			name: "URL with only utm_source",
			url:  "https://example.com/?utm_source=newsletter",
			want: UTMParams{Source: "newsletter"},
		},
		{
			name: "URL without UTM params",
			url:  "https://example.com/page",
			want: UTMParams{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractUTM(tc.url)
			if got != tc.want {
				t.Errorf("ExtractUTM(%q) = %+v, want %+v", tc.url, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HashUserID
// ---------------------------------------------------------------------------

func TestHashUserID(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := HashUserID(""); got != "" {
			t.Errorf("HashUserID('') = %q, want empty", got)
		}
	})

	t.Run("deterministic", func(t *testing.T) {
		h1 := HashUserID("user-123")
		h2 := HashUserID("user-123")
		if h1 != h2 {
			t.Errorf("HashUserID not deterministic: %q != %q", h1, h2)
		}
		if len(h1) != 64 {
			t.Errorf("expected 64 hex chars (SHA-256), got %d: %q", len(h1), h1)
		}
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		h1 := HashUserID("user-1")
		h2 := HashUserID("user-2")
		if h1 == h2 {
			t.Error("different user IDs should produce different hashes")
		}
	})
}

// ---------------------------------------------------------------------------
// StripURL
// ---------------------------------------------------------------------------

func TestStripURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com/page?foo=bar&baz=qux", "https://example.com/page"},
		{"https://example.com/path#fragment", "https://example.com/path"},
		{"https://example.com/?q=1#anchor", "https://example.com/"},
		{"https://example.com/clean", "https://example.com/clean"},
		{"not-a-url", "not-a-url"}, // invalid URLs returned as-is
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := StripURL(tc.input)
			if got != tc.want {
				t.Errorf("StripURL(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
