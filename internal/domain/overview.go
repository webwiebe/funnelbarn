package domain

// ProjectRollup is a per-project summary row for the overview dashboard.
type ProjectRollup struct {
	ProjectID      string `json:"project_id"`
	Events         int64  `json:"events"`
	UniqueSessions int64  `json:"unique_sessions"`
}

// ProjectDayCount is a (bucket, project) unique-session count for the per-site
// chart. Day holds "YYYY-MM-DD" for daily buckets and "YYYY-MM-DDTHH:00:00Z"
// for hourly buckets.
type ProjectDayCount struct {
	Day       string `json:"day"`
	ProjectID string `json:"project_id"`
	Count     int64  `json:"count"`
}

// OverviewPageStat is a top page attributed to its project.
type OverviewPageStat struct {
	ProjectID string `json:"project_id"`
	URL       string `json:"url"`
	Views     int64  `json:"views"`
}

// OverviewReferrerStat is a top referrer attributed to its project.
type OverviewReferrerStat struct {
	ProjectID string `json:"project_id"`
	Domain    string `json:"domain"`
	Visits    int64  `json:"visits"`
}

// OverviewCountryStat is a top country attributed to its project.
type OverviewCountryStat struct {
	ProjectID   string `json:"project_id"`
	CountryCode string `json:"country_code"`
	Count       int64  `json:"count"`
}

// DimensionStat is a single value+count for a dimension breakdown.
type DimensionStat struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}
