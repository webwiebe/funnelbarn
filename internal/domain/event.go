package domain

import "time"

// Event represents a single analytics event.
type Event struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"project_id"`
	SessionID      string    `json:"session_id"`
	UserIDHash     string    `json:"user_id_hash"`
	Name           string    `json:"name"`
	URL            string    `json:"url"`
	Referrer       string    `json:"referrer"`
	ReferrerDomain string    `json:"referrer_domain"`
	UTMSource      string    `json:"utm_source"`
	UTMMedium      string    `json:"utm_medium"`
	UTMCampaign    string    `json:"utm_campaign"`
	UTMTerm        string    `json:"utm_term"`
	UTMContent     string    `json:"utm_content"`
	Properties     string    `json:"properties"`
	UserAgent      string    `json:"user_agent"`
	Browser        string    `json:"browser"`
	OS             string    `json:"os"`
	DeviceType     string    `json:"device_type"`
	CountryCode    string    `json:"country_code"`
	PageViewID     string    `json:"page_view_id,omitempty"`
	IngestID       string    `json:"ingest_id"`
	OccurredAt     time.Time `json:"occurred_at"`
	CreatedAt      time.Time `json:"created_at"`
	Environment    string    `json:"environment,omitempty"`

	// ClientIP is transient — used to pass the real IP to UpsertSession. Not stored.
	ClientIP string `json:"-"`
	// SessionSignalsRaw is transient — device/browser signals from the SDK payload.
	// Passed to UpsertSessionSignals; not stored directly on the event.
	SessionSignalsRaw map[string]any `json:"-"`
}

// EventNameStat is an event name with count.
type EventNameStat struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// PageStat is a page URL with view count.
type PageStat struct {
	URL   string `json:"url"`
	Views int64  `json:"views"`
}

// ReferrerStat is a referrer domain with visit count.
type ReferrerStat struct {
	Domain string `json:"domain"`
	Visits int64  `json:"visits"`
}

// TimeSeriesPoint is a timestamp+count pair.
type TimeSeriesPoint struct {
	Time  string `json:"time"`
	Count int64  `json:"count"`
}

// UTMStat is a UTM value with count.
type UTMStat struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// BrowserStat is a browser name with count.
type BrowserStat struct {
	Browser string `json:"browser"`
	Count   int64  `json:"count"`
}

// DeviceStat is a device type with count.
type DeviceStat struct {
	DeviceType string `json:"device_type"`
	Count      int64  `json:"count"`
}

// CountryStat is a country code with count.
type CountryStat struct {
	CountryCode string `json:"country_code"`
	Count       int64  `json:"count"`
}

// OSStat is an OS name with count.
type OSStat struct {
	OS    string `json:"os"`
	Count int64  `json:"count"`
}

// EventCountFilter narrows the event-counts and event-breakdown queries. An
// empty field matches everything.
type EventCountFilter struct {
	Environment string
	// ExcludeBots drops events whose device_type is "bot".
	ExcludeBots bool
	// PropKey and PropValue keep events whose JSON property PropKey equals
	// PropValue. PropKey must be a plain identifier (see ValidPropertyKey).
	PropKey   string
	PropValue string // "true" and "false" also match JSON booleans
}

// EventListFilter narrows ListEventsFiltered. An empty field matches everything and
// a zero From or To leaves that side of the window open.
type EventListFilter struct {
	Name        string
	Environment string
	From        time.Time
	To          time.Time
}

// EventFilter narrows a cross-project event listing. Empty fields are ignored.
type EventFilter struct {
	ProjectID   string
	Name        string
	Environment string
	// Keyset cursor: return rows strictly older than (CursorOccurredAt, CursorID).
	// Zero time means "from the newest".
	CursorOccurredAt time.Time
	CursorID         string
}

// BreakdownRow is one value of a breakdown dimension.
type BreakdownRow struct {
	Value    string `json:"value"`
	Count    int64  `json:"count"`
	Sessions int64  `json:"sessions"`
}

// BreakdownDimensions are the columns EventBreakdown can group by.
var BreakdownDimensions = map[string]string{
	"device_type":     "device_type",
	"referrer_domain": "referrer_domain",
	"utm_source":      "utm_source",
}

// ValidPropertyKey reports whether key can be used as a JSON property filter:
// letters, digits and underscores only, so it cannot alter the JSON path.
func ValidPropertyKey(key string) bool {
	if key == "" || len(key) > 64 {
		return false
	}
	for _, r := range key {
		if r != '_' && (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

// DistributionEntry is one value in a field distribution breakdown.
type DistributionEntry struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
	Pct   int    `json:"pct"`
}
