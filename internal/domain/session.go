package domain

import "time"

// Session represents an analytics session.
type Session struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	EventCount  int       `json:"event_count"`
	EntryURL    string    `json:"entry_url"`
	ExitURL     string    `json:"exit_url"`
	Referrer    string    `json:"referrer"`
	UTMSource   string    `json:"utm_source"`
	UTMMedium   string    `json:"utm_medium"`
	UTMCampaign string    `json:"utm_campaign"`
	DeviceType  string    `json:"device_type"`
	CountryCode string    `json:"country_code"`
	Environment string    `json:"environment,omitempty"`

	// Geo fields — populated from IP on first event, nullable, clearable on request.
	IP              string  `json:"ip,omitempty"`
	City            string  `json:"city,omitempty"`
	Region          string  `json:"region,omitempty"`
	Latitude        float64 `json:"latitude,omitempty"`
	Longitude       float64 `json:"longitude,omitempty"`
	Timezone        string  `json:"timezone,omitempty"`
	ASNOrg          string  `json:"asn_org,omitempty"`
	ConnectionClass string  `json:"connection_class,omitempty"`
	GeoAnonymized   bool    `json:"geo_anonymized,omitempty"`
}

// SessionSignals holds the device/browser signals collected by the SDK.
type SessionSignals struct {
	ScreenWidth     *int
	ScreenHeight    *int
	PixelRatio      *float64
	Touch           *bool
	DarkMode        *bool
	ReducedMotion   *bool
	BrowserTimezone string
	CPUCores        *int
}
