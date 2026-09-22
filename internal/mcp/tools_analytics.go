package mcp

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wiebe-xyz/funnelbarn/internal/repository"
)

// Limits used by the analytics tools below. Named so a tool's description
// and its validation stay in sync.
const (
	eventCountsDefaultLimit  = 100 // get_event_counts: default when names is empty
	eventCountsMaxLimit      = 500 // get_event_counts: matches handleEventCounts' cap
	listEventsDefaultLimit   = 50  // list_events: matches handleListEvents' default
	listEventsMaxLimit       = 500 // list_events: matches handleListEvents' cap
	eventPropertyValuesLimit = 50  // list_event_property_values: matches handleEventPropertyValues' fixed limit
)

// registerAnalyticsTools registers seven read-only tools that surface a
// project's event analytics: aggregate stats, event names and counts, raw
// events, property keys and values, and recorded environments. Each is
// modeled on the internal/api handler named in its description: same
// service calls, same caps/limits, same environment filter semantics (an
// empty environment means every environment).
//
// Every tool takes an optional `project` argument and resolves it with
// Call.Project. Time-ranged tools use parseRange for `from`/`to` (default:
// the last 7 days) and always bucket time series by day. The REST
// dashboard only buckets hourly when its `range=24h`/`range=7d` shorthand is
// set, which these tools have no equivalent of, so they match its default
// (unshorthanded) behavior.
func registerAnalyticsTools(s *mcp.Server, d *Deps) {
	addTool(s, d, &mcp.Tool{
		Name: "get_stats",
		Description: "Aggregate analytics for a project over a time range, modeled on the " +
			"dashboard (handleDashboard): total events, unique sessions, bounce rate, average " +
			"events per session, top pages/referrers/browsers, device types, top event names, top " +
			"UTM sources, and daily time series for events and sessions. Defaults to the last 7 " +
			"days; pass `environment` to restrict to one recorded environment.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, getStats)

	addTool(s, d, &mcp.Tool{
		Name: "list_event_names",
		Description: "List every distinct event name a project has recorded (handleEventNames), " +
			"for matching a real event name before defining a funnel step or segment rule.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, listEventNames)

	addTool(s, d, &mcp.Tool{
		Name: "get_event_counts",
		Description: "Per-event-name counts over a time range (handleEventCounts): how many " +
			"times each event fired. Defaults to the last 7 days and the top 100 event names by " +
			"count; raise `limit` (max 500) to see more, or pass `names` to restrict to specific " +
			"event names. Names are matched among the top 500 by count in range, so a name " +
			"outside the top 500 for this project won't appear.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, getEventCounts)

	addTool(s, d, &mcp.Tool{
		Name: "list_events",
		Description: "List the most recent raw events for a project (handleListEvents), newest " +
			"first, with full metadata (URL, referrer, UTM params, browser/device/OS, properties). " +
			"Not filterable by name or date. Capped at 500 per call (default 50); page with " +
			"`offset` (pass the previous call's offset+limit).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, listEvents)

	addTool(s, d, &mcp.Tool{
		Name: "list_event_properties",
		Description: "List the property keys recorded on events with the given name " +
			"(handleEventProperties): both custom JSON properties and populated built-in metadata " +
			"columns (browser, os, device_type, etc.). Use before defining a segment rule so it " +
			"matches a real property.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, listEventProperties)

	addTool(s, d, &mcp.Tool{
		Name: "list_event_property_values",
		Description: "List up to 50 distinct values a property has taken on events with the " +
			"given name (handleEventPropertyValues), for matching a real value before defining a " +
			"segment rule.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, listEventPropertyValues)

	addTool(s, d, &mcp.Tool{
		Name: "list_environments",
		Description: "List the canonical environment values a project has recorded events under " +
			"(handleEnvironments, e.g. production, staging), for use as the `environment` argument " +
			"on the other analytics tools.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, scopeRead, listEnvironments)
}

// --- get_stats ---

type getStatsIn struct {
	Project     string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	From        string `json:"from,omitempty" jsonschema:"Start of the range, RFC 3339 or YYYY-MM-DD. Defaults to 7 days before to."`
	To          string `json:"to,omitempty" jsonschema:"End of the range, RFC 3339 or YYYY-MM-DD. Defaults to now."`
	Environment string `json:"environment,omitempty" jsonschema:"Restrict to one recorded environment (see list_environments); omit for every environment."`
}

type getStatsOut struct {
	Project             string                       `json:"project" jsonschema:"Slug of the project these stats are for."`
	From                time.Time                    `json:"from" jsonschema:"Start of the range (inclusive)."`
	To                  time.Time                    `json:"to" jsonschema:"End of the range (inclusive)."`
	Environment         string                       `json:"environment,omitempty" jsonschema:"The environment filter applied, empty if none."`
	TotalEvents         int64                        `json:"total_events" jsonschema:"Total events in range."`
	UniqueSessions      int64                        `json:"unique_sessions" jsonschema:"Distinct sessions in range."`
	BounceRate          float64                      `json:"bounce_rate" jsonschema:"Share of sessions with exactly one event, 0-1."`
	AvgEventsPerSession float64                      `json:"avg_events_per_session" jsonschema:"Average events per session."`
	TopPages            []repository.PageStat        `json:"top_pages" jsonschema:"Up to 10 most-viewed pages."`
	TopReferrers        []repository.ReferrerStat    `json:"top_referrers" jsonschema:"Up to 10 top referrer domains."`
	TopBrowsers         []repository.BrowserStat     `json:"top_browsers" jsonschema:"Up to 5 top browsers."`
	DeviceTypes         []repository.DeviceStat      `json:"device_types" jsonschema:"Breakdown by device type."`
	TopEventNames       []repository.EventNameStat   `json:"top_event_names" jsonschema:"Up to 10 most frequent event names."`
	TopUTMSources       []repository.UTMStat         `json:"top_utm_sources" jsonschema:"Up to 5 top UTM sources."`
	EventsTimeSeries    []repository.TimeSeriesPoint `json:"events_time_series" jsonschema:"Daily event counts across the range."`
	SessionsTimeSeries  []repository.TimeSeriesPoint `json:"sessions_time_series" jsonschema:"Daily unique-session counts across the range."`
}

func getStats(ctx context.Context, c *Call, in getStatsIn) (getStatsOut, error) {
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return getStatsOut{}, err
	}
	from, to, err := parseRange(in.From, in.To, time.Now())
	if err != nil {
		return getStatsOut{}, err
	}
	ev, env := c.Deps.Events, in.Environment
	out := getStatsOut{Project: p.Slug, From: from, To: to, Environment: env}

	// Each query fills one field; the first failure aborts the tool.
	queries := []func() error{
		func() (err error) { out.TotalEvents, err = ev.CountEvents(ctx, p.ID, from, to, env); return },
		func() (err error) { out.UniqueSessions, err = ev.UniqueSessionCount(ctx, p.ID, from, to, env); return },
		func() (err error) { out.BounceRate, err = ev.BounceRate(ctx, p.ID, from, to, env); return },
		func() (err error) {
			out.AvgEventsPerSession, err = ev.AvgEventsPerSession(ctx, p.ID, from, to, env)
			return
		},
		func() (err error) { out.TopPages, err = ev.TopPages(ctx, p.ID, from, to, 10, env); return },
		func() (err error) { out.TopReferrers, err = ev.TopReferrers(ctx, p.ID, from, to, 10, env); return },
		func() (err error) { out.TopBrowsers, err = ev.TopBrowsers(ctx, p.ID, from, to, 5, env); return },
		func() (err error) { out.DeviceTypes, err = ev.TopDeviceTypes(ctx, p.ID, from, to, env); return },
		func() (err error) { out.TopEventNames, err = ev.TopEventNames(ctx, p.ID, from, to, 10, env); return },
		func() (err error) { out.TopUTMSources, err = ev.TopUTMSources(ctx, p.ID, from, to, 5, env); return },
		func() (err error) { out.EventsTimeSeries, err = ev.DailyEventCounts(ctx, p.ID, from, to, env); return },
		func() (err error) {
			out.SessionsTimeSeries, err = ev.DailyUniqueSessions(ctx, p.ID, from, to, env)
			return
		},
	}
	for _, q := range queries {
		if err := q(); err != nil {
			return getStatsOut{}, err
		}
	}

	out.TopPages = nonNil(out.TopPages)
	out.TopReferrers = nonNil(out.TopReferrers)
	out.TopBrowsers = nonNil(out.TopBrowsers)
	out.DeviceTypes = nonNil(out.DeviceTypes)
	out.TopEventNames = nonNil(out.TopEventNames)
	out.TopUTMSources = nonNil(out.TopUTMSources)
	out.EventsTimeSeries = nonNil(out.EventsTimeSeries)
	out.SessionsTimeSeries = nonNil(out.SessionsTimeSeries)
	return out, nil
}

// --- list_event_names ---

type listEventNamesIn struct {
	Project string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
}

type listEventNamesOut struct {
	Project    string   `json:"project" jsonschema:"Slug of the project these names are from."`
	EventNames []string `json:"event_names" jsonschema:"Every distinct event name recorded, alphabetical."`
}

func listEventNames(ctx context.Context, c *Call, in listEventNamesIn) (listEventNamesOut, error) {
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return listEventNamesOut{}, err
	}
	names, err := c.Deps.Events.DistinctEventNames(ctx, p.ID)
	if err != nil {
		return listEventNamesOut{}, err
	}
	return listEventNamesOut{Project: p.Slug, EventNames: nonNil(names)}, nil
}

// --- get_event_counts ---

type getEventCountsIn struct {
	Project     string   `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	From        string   `json:"from,omitempty" jsonschema:"Start of the range, RFC 3339 or YYYY-MM-DD. Defaults to 7 days before to."`
	To          string   `json:"to,omitempty" jsonschema:"End of the range, RFC 3339 or YYYY-MM-DD. Defaults to now."`
	Environment string   `json:"environment,omitempty" jsonschema:"Restrict to one recorded environment; omit for every environment."`
	Names       []string `json:"names,omitempty" jsonschema:"Restrict to these event names (matched among the top 500 by count in range); omit for every event name up to limit."`
	Limit       int      `json:"limit,omitempty" jsonschema:"Max distinct event names returned when names is empty, 1-500 (default 100)."`
}

type getEventCountsOut struct {
	Project     string                     `json:"project" jsonschema:"Slug of the project these counts are from."`
	From        time.Time                  `json:"from" jsonschema:"Start of the range (inclusive)."`
	To          time.Time                  `json:"to" jsonschema:"End of the range (inclusive)."`
	Environment string                     `json:"environment,omitempty" jsonschema:"The environment filter applied, empty if none."`
	Events      []repository.EventNameStat `json:"events" jsonschema:"Event name and count, most frequent first."`
	TotalEvents int64                      `json:"total_events" jsonschema:"Sum of count across the returned events."`
}

func getEventCounts(ctx context.Context, c *Call, in getEventCountsIn) (getEventCountsOut, error) {
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return getEventCountsOut{}, err
	}
	from, to, err := parseRange(in.From, in.To, time.Now())
	if err != nil {
		return getEventCountsOut{}, err
	}

	limit := in.Limit
	if limit == 0 {
		limit = eventCountsDefaultLimit
	}
	if limit < 1 || limit > eventCountsMaxLimit {
		return getEventCountsOut{}, invalidInput("limit must be between 1 and %d, got %d", eventCountsMaxLimit, limit)
	}

	// A names filter is applied client-side (TopEventNames has no such
	// parameter), so widen the fetch to the service's own cap: otherwise a
	// requested name ranked below `limit` by count would never be seen.
	fetchLimit := limit
	if len(in.Names) > 0 {
		fetchLimit = eventCountsMaxLimit
	}

	counts, err := c.Deps.Events.TopEventNames(ctx, p.ID, from, to, fetchLimit, in.Environment)
	if err != nil {
		return getEventCountsOut{}, err
	}

	if len(in.Names) > 0 {
		counts = keepEventNames(counts, in.Names)
	}

	var total int64
	for _, es := range counts {
		total += es.Count
	}

	return getEventCountsOut{
		Project:     p.Slug,
		From:        from,
		To:          to,
		Environment: in.Environment,
		Events:      nonNil(counts),
		TotalEvents: total,
	}, nil
}

// keepEventNames returns the entries of counts whose name is in names, in
// their original order.
func keepEventNames(counts []repository.EventNameStat, names []string) []repository.EventNameStat {
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	kept := make([]repository.EventNameStat, 0, len(counts))
	for _, es := range counts {
		if want[es.Name] {
			kept = append(kept, es)
		}
	}
	return kept
}

// --- list_events ---

type listEventsIn struct {
	Project string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	Limit   int    `json:"limit,omitempty" jsonschema:"Events per page, 1-500 (default 50); out-of-range values fall back to the default."`
	Offset  int    `json:"offset,omitempty" jsonschema:"Events to skip, for paging; negative values fall back to 0."`
}

type listEventsOut struct {
	Project string             `json:"project" jsonschema:"Slug of the project these events are from."`
	Events  []repository.Event `json:"events" jsonschema:"Events, newest first."`
	Limit   int                `json:"limit" jsonschema:"Page size used."`
	Offset  int                `json:"offset" jsonschema:"Offset used."`
	HasMore bool               `json:"has_more" jsonschema:"Whether another page may exist; pass offset+limit as the next call's offset."`
}

func listEvents(ctx context.Context, c *Call, in listEventsIn) (listEventsOut, error) {
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return listEventsOut{}, err
	}

	limit := listEventsDefaultLimit
	if in.Limit > 0 && in.Limit <= listEventsMaxLimit {
		limit = in.Limit
	}
	offset := 0
	if in.Offset >= 0 {
		offset = in.Offset
	}

	events, err := c.Deps.Events.ListEvents(ctx, p.ID, limit, offset)
	if err != nil {
		return listEventsOut{}, err
	}

	return listEventsOut{
		Project: p.Slug,
		Events:  nonNil(events),
		Limit:   limit,
		Offset:  offset,
		HasMore: len(events) == limit,
	}, nil
}

// --- list_event_properties ---

type listEventPropertiesIn struct {
	Project   string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	EventName string `json:"event_name" jsonschema:"Event name to list property keys for (see list_event_names)."`
}

type listEventPropertiesOut struct {
	Project    string   `json:"project" jsonschema:"Slug of the project these properties are from."`
	EventName  string   `json:"event_name" jsonschema:"The event name queried."`
	Properties []string `json:"properties" jsonschema:"Property keys recorded on this event: custom JSON properties plus populated built-in metadata columns."`
}

func listEventProperties(ctx context.Context, c *Call, in listEventPropertiesIn) (listEventPropertiesOut, error) {
	if in.EventName == "" {
		return listEventPropertiesOut{}, invalidInput("event_name is required")
	}
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return listEventPropertiesOut{}, err
	}

	props, err := c.Deps.Events.DistinctEventProperties(ctx, p.ID, in.EventName)
	if err != nil {
		return listEventPropertiesOut{}, err
	}
	populated, err := c.Deps.Events.PopulatedMetadataColumns(ctx, p.ID, in.EventName)
	if err != nil {
		return listEventPropertiesOut{}, err
	}

	all := append(populated, props...)
	return listEventPropertiesOut{Project: p.Slug, EventName: in.EventName, Properties: nonNil(all)}, nil
}

// --- list_event_property_values ---

type listEventPropertyValuesIn struct {
	Project   string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
	EventName string `json:"event_name" jsonschema:"Event name the property belongs to (see list_event_names)."`
	Property  string `json:"property" jsonschema:"Property key to list values for (see list_event_properties)."`
}

type listEventPropertyValuesOut struct {
	Project   string   `json:"project" jsonschema:"Slug of the project these values are from."`
	EventName string   `json:"event_name" jsonschema:"The event name queried."`
	Property  string   `json:"property" jsonschema:"The property key queried."`
	Values    []string `json:"values" jsonschema:"Up to 50 distinct values recorded for this property."`
}

func listEventPropertyValues(ctx context.Context, c *Call, in listEventPropertyValuesIn) (listEventPropertyValuesOut, error) {
	if in.EventName == "" {
		return listEventPropertyValuesOut{}, invalidInput("event_name is required")
	}
	if in.Property == "" {
		return listEventPropertyValuesOut{}, invalidInput("property is required")
	}
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return listEventPropertyValuesOut{}, err
	}

	vals, err := c.Deps.Events.DistinctPropertyValues(ctx, p.ID, in.EventName, in.Property, eventPropertyValuesLimit)
	if err != nil {
		return listEventPropertyValuesOut{}, err
	}
	return listEventPropertyValuesOut{Project: p.Slug, EventName: in.EventName, Property: in.Property, Values: nonNil(vals)}, nil
}

// --- list_environments ---

type listEnvironmentsIn struct {
	Project string `json:"project,omitempty" jsonschema:"Project slug or ID; defaults to the repository's project from the x-funnelbarn-project header."`
}

type listEnvironmentsOut struct {
	Project      string   `json:"project" jsonschema:"Slug of the project these environments are from."`
	Environments []string `json:"environments" jsonschema:"Canonical environment values recorded for this project, alphabetical."`
}

func listEnvironments(ctx context.Context, c *Call, in listEnvironmentsIn) (listEnvironmentsOut, error) {
	p, err := c.Project(ctx, in.Project)
	if err != nil {
		return listEnvironmentsOut{}, err
	}
	envs, err := c.Deps.Events.DistinctEnvironments(ctx, p.ID)
	if err != nil {
		return listEnvironmentsOut{}, err
	}
	return listEnvironmentsOut{Project: p.Slug, Environments: nonNil(envs)}, nil
}
