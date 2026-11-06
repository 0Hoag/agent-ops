package insight

import "time"

type DeliveryFilter struct {
	RepositoryID string
	From, To     *time.Time
}

type DeliveryResponse struct {
	From         string             `json:"from"`
	To           string             `json:"to"`
	History      DeliveryHistory    `json:"history"`
	MergedPRs    DeliveryCostMetric `json:"merged_prs"`
	ClosedIssues DeliveryCostMetric `json:"closed_issues"`
	SendBack     SendBackMetric     `json:"send_back"`
	FirstCI      FirstCIMetric      `json:"first_ci_success"`
	IssueToMerge LeadTimeMetric     `json:"issue_to_merge_lead_time"`
	Harness      []HarnessMetric    `json:"harness_comparison"`
}
type DeliveryHistory struct {
	Complete    bool `json:"complete"`
	Unavailable int  `json:"unavailable"`
}
type DeliveryCostMetric struct {
	Count             int      `json:"count"`
	Eligible          int      `json:"eligible"`
	Unavailable       int      `json:"unavailable"`
	ActualCostUSD     *float64 `json:"actual_cost_usd"`
	EquivalentCostUSD *float64 `json:"equivalent_cost_usd"`
	CostPerItemUSD    *float64 `json:"cost_per_item_usd"`
}
type LeadTimeMetric struct {
	Completed     int      `json:"completed"`
	Unavailable   int      `json:"unavailable"`
	MedianSeconds *float64 `json:"median_seconds"`
}
type HarnessMetric struct {
	Harness               string   `json:"harness"`
	Runs                  int      `json:"runs"`
	ChangedLines          int64    `json:"changed_lines"`
	Eligible              int      `json:"eligible"`
	Unavailable           int      `json:"unavailable"`
	EquivalentCostUSD     *float64 `json:"equivalent_cost_usd"`
	CostPerChangedLineUSD *float64 `json:"cost_per_changed_line_usd"`
}
