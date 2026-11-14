package domain

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
