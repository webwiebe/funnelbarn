package repository

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/wiebe-xyz/funnelbarn/internal/tracing"
)

// FlowNode is a node in the page flow Sankey graph.
type FlowNode struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	NodeType string `json:"type"` // "page", "referrer", "exit"
	Depth    int    `json:"depth"`
	Sessions int64  `json:"sessions"`
}

// FlowLink is a directed edge in the page flow Sankey graph.
type FlowLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Value  int64  `json:"value"`
}

// PageFlowResult is the complete page flow graph centered on a focused page.
type PageFlowResult struct {
	FocusedPage   string     `json:"focused_page"`
	TotalSessions int64      `json:"total_sessions"`
	Nodes         []FlowNode `json:"nodes"`
	Links         []FlowLink `json:"links"`
}

// PageFlows returns a Sankey flow graph centered on the given page within the
// time range. If page is empty the most-visited page is used. depth controls
// how many hops before and after the focused page are included (max 10).
func (s *Store) PageFlows(ctx context.Context, projectID, page string, depth int, from, to time.Time) (PageFlowResult, error) {
	ctx, span := tracing.StartSpan(ctx, "repository.PageFlows",
		attribute.String("project.id", projectID),
		attribute.String("page", page),
		attribute.Int("depth", depth),
	)
	defer span.End()

	if depth <= 0 || depth > 10 {
		depth = 5
	}

	if page == "" {
		top, err := s.TopPages(ctx, projectID, from, to, 1)
		if err != nil {
			tracing.RecordError(span, err)
			return PageFlowResult{}, err
		}
		if len(top) == 0 {
			return PageFlowResult{Nodes: []FlowNode{}, Links: []FlowLink{}}, nil
		}
		page = top[0].URL
		span.SetAttributes(attribute.String("page.resolved", page))
	}

	totalSessions, err := s.flowTotalSessions(ctx, projectID, page, from, to)
	if err != nil {
		tracing.RecordError(span, err)
		return PageFlowResult{}, err
	}
	if totalSessions == 0 {
		return PageFlowResult{FocusedPage: page, Nodes: []FlowNode{}, Links: []FlowLink{}}, nil
	}

	transitions, err := s.flowTransitions(ctx, projectID, page, depth, from, to)
	if err != nil {
		tracing.RecordError(span, err)
		return PageFlowResult{}, err
	}

	exitCount, err := s.flowExitCount(ctx, projectID, page, from, to)
	if err != nil {
		tracing.RecordError(span, err)
		return PageFlowResult{}, err
	}

	entryReferrers, err := s.flowEntryReferrers(ctx, projectID, page, from, to)
	if err != nil {
		tracing.RecordError(span, err)
		return PageFlowResult{}, err
	}

	span.SetAttributes(attribute.Int64("total_sessions", totalSessions))
	return buildFlowResult(page, totalSessions, exitCount, transitions, entryReferrers), nil
}

type flowTransition struct {
	SourceURL   string
	SourceDepth int
	TargetURL   string
	TargetDepth int
	Sessions    int64
}

type flowEntryReferrer struct {
	Domain   string
	Sessions int64
}

func (s *Store) flowTotalSessions(ctx context.Context, projectID, page string, from, to time.Time) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "repository.flows.totalSessions")
	defer span.End()

	const q = `
SELECT COUNT(DISTINCT session_id)
FROM events
WHERE project_id = ?
    AND name = 'page_view'
    AND occurred_at >= ? AND occurred_at <= ?
    AND url = ?`
	var n int64
	err := s.db.QueryRowContext(ctx, q, projectID, from, to, page).Scan(&n)
	if err != nil {
		tracing.RecordError(span, err)
	}
	return n, err
}

func (s *Store) flowTransitions(ctx context.Context, projectID, page string, depth int, from, to time.Time) ([]flowTransition, error) {
	ctx, span := tracing.StartSpan(ctx, "repository.flows.transitions",
		attribute.Int("depth", depth),
	)
	defer span.End()

	q := fmt.Sprintf(`
WITH page_seq AS (
    SELECT session_id, COALESCE(url, '') AS url,
        ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY occurred_at) AS pos
    FROM events
    WHERE project_id = ?
        AND name = 'page_view'
        AND occurred_at >= ? AND occurred_at <= ?
        AND url IS NOT NULL AND url != ''
),
focused AS (
    SELECT session_id, MIN(pos) AS target_pos
    FROM page_seq
    WHERE url = ?
    GROUP BY session_id
),
ctx AS (
    SELECT ps.session_id, ps.url, ps.pos - f.target_pos AS depth
    FROM focused f
    JOIN page_seq ps ON ps.session_id = f.session_id
    WHERE ps.pos - f.target_pos BETWEEN -%d AND %d
)
SELECT a.url, a.depth, b.url, b.depth, COUNT(DISTINCT a.session_id)
FROM ctx a
JOIN ctx b ON a.session_id = b.session_id AND b.depth = a.depth + 1
GROUP BY a.url, a.depth, b.url, b.depth
ORDER BY COUNT(DISTINCT a.session_id) DESC
LIMIT 500`, depth, depth)

	rows, err := s.db.QueryContext(ctx, q, projectID, from, to, page)
	if err != nil {
		tracing.RecordError(span, err)
		return nil, err
	}
	defer rows.Close()

	var out []flowTransition
	for rows.Next() {
		var t flowTransition
		if err := rows.Scan(&t.SourceURL, &t.SourceDepth, &t.TargetURL, &t.TargetDepth, &t.Sessions); err != nil {
			tracing.RecordError(span, err)
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		tracing.RecordError(span, err)
		return nil, err
	}
	span.SetAttributes(attribute.Int("transitions.count", len(out)))
	return out, nil
}

func (s *Store) flowExitCount(ctx context.Context, projectID, page string, from, to time.Time) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "repository.flows.exitCount")
	defer span.End()

	const q = `
WITH page_seq AS (
    SELECT session_id, COALESCE(url, '') AS url,
        ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY occurred_at) AS pos
    FROM events
    WHERE project_id = ?
        AND name = 'page_view'
        AND occurred_at >= ? AND occurred_at <= ?
        AND url IS NOT NULL AND url != ''
),
focused AS (
    SELECT session_id, MIN(pos) AS target_pos
    FROM page_seq
    WHERE url = ?
    GROUP BY session_id
)
SELECT COUNT(DISTINCT f.session_id)
FROM focused f
LEFT JOIN page_seq nxt
    ON nxt.session_id = f.session_id AND nxt.pos = f.target_pos + 1
WHERE nxt.session_id IS NULL`
	var n int64
	err := s.db.QueryRowContext(ctx, q, projectID, from, to, page).Scan(&n)
	if err != nil {
		tracing.RecordError(span, err)
	}
	return n, err
}

func (s *Store) flowEntryReferrers(ctx context.Context, projectID, page string, from, to time.Time) ([]flowEntryReferrer, error) {
	ctx, span := tracing.StartSpan(ctx, "repository.flows.entryReferrers")
	defer span.End()

	const q = `
WITH page_seq AS (
    SELECT session_id, COALESCE(url, '') AS url,
        COALESCE(referrer_domain, '') AS referrer_domain,
        ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY occurred_at) AS pos
    FROM events
    WHERE project_id = ?
        AND name = 'page_view'
        AND occurred_at >= ? AND occurred_at <= ?
        AND url IS NOT NULL AND url != ''
),
focused AS (
    SELECT session_id, MIN(pos) AS target_pos
    FROM page_seq
    WHERE url = ?
    GROUP BY session_id
)
SELECT
    CASE WHEN ps.referrer_domain != '' THEN ps.referrer_domain ELSE '(direct)' END AS referrer,
    COUNT(DISTINCT f.session_id)
FROM focused f
JOIN page_seq ps ON ps.session_id = f.session_id AND ps.pos = 1
WHERE f.target_pos = 1
GROUP BY referrer
ORDER BY COUNT(DISTINCT f.session_id) DESC
LIMIT 20`

	rows, err := s.db.QueryContext(ctx, q, projectID, from, to, page)
	if err != nil {
		tracing.RecordError(span, err)
		return nil, err
	}
	defer rows.Close()

	var out []flowEntryReferrer
	for rows.Next() {
		var r flowEntryReferrer
		if err := rows.Scan(&r.Domain, &r.Sessions); err != nil {
			tracing.RecordError(span, err)
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		tracing.RecordError(span, err)
		return nil, err
	}
	return out, nil
}

func flowNodeID(url string, depth int) string {
	return fmt.Sprintf("%s|%d", url, depth)
}

func flowReferrerNodeID(domain string) string {
	return fmt.Sprintf("referrer:%s|-1", domain)
}

func buildFlowResult(page string, totalSessions, exitCount int64, transitions []flowTransition, entryReferrers []flowEntryReferrer) PageFlowResult {
	nodeMap := map[string]FlowNode{}

	upsert := func(id, label, nodeType string, depth int, sessions int64) {
		if existing, ok := nodeMap[id]; ok {
			if sessions > existing.Sessions {
				existing.Sessions = sessions
				nodeMap[id] = existing
			}
		} else {
			nodeMap[id] = FlowNode{ID: id, Label: label, NodeType: nodeType, Depth: depth, Sessions: sessions}
		}
	}

	upsert(flowNodeID(page, 0), page, "page", 0, totalSessions)

	var links []FlowLink

	for _, t := range transitions {
		srcID := flowNodeID(t.SourceURL, t.SourceDepth)
		tgtID := flowNodeID(t.TargetURL, t.TargetDepth)
		upsert(srcID, t.SourceURL, "page", t.SourceDepth, t.Sessions)
		upsert(tgtID, t.TargetURL, "page", t.TargetDepth, t.Sessions)
		links = append(links, FlowLink{Source: srcID, Target: tgtID, Value: t.Sessions})
	}

	for _, r := range entryReferrers {
		id := flowReferrerNodeID(r.Domain)
		upsert(id, r.Domain, "referrer", -1, r.Sessions)
		links = append(links, FlowLink{Source: id, Target: flowNodeID(page, 0), Value: r.Sessions})
	}

	if exitCount > 0 {
		const exitID = "exit|1"
		upsert(exitID, "(drop-off)", "exit", 1, exitCount)
		links = append(links, FlowLink{Source: flowNodeID(page, 0), Target: exitID, Value: exitCount})
	}

	nodes := make([]FlowNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}

	if links == nil {
		links = []FlowLink{}
	}

	return PageFlowResult{
		FocusedPage:   page,
		TotalSessions: totalSessions,
		Nodes:         nodes,
		Links:         links,
	}
}
