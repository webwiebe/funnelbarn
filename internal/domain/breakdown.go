package domain

import "slices"

// PropertyBreakdown is one row of a top-N breakdown result.
type PropertyBreakdown struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// metadataColumns are the events columns a breakdown reads directly instead of
// extracting from the JSON properties.
var metadataColumns = []string{
	"url", "referrer", "referrer_domain",
	"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content",
	"browser", "os", "device_type", "country_code",
}

// IsMetadataColumn reports whether name is an events metadata column.
func IsMetadataColumn(name string) bool {
	return slices.Contains(metadataColumns, name)
}

// MetadataColumns returns the events metadata columns in their fixed order.
func MetadataColumns() []string {
	return metadataColumns
}
