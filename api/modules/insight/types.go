package insight

import "time"

type Response struct {
	RepositoryID string         `json:"repository_id"`
	History      History        `json:"history"`
	SendBack     SendBackMetric `json:"send_back"`
	FirstCI      FirstCIMetric  `json:"first_ci_success"`
	CycleTime    CycleMetric    `json:"cycle_time"`
	Suggestions  []Suggestion   `json:"suggestions"`
}

type History struct {
	Complete bool   `json:"complete"`
	Reason   string `json:"reason,omitempty"`
}
type SendBackMetric struct {
	Count       int        `json:"count"`
	Rate        float64    `json:"rate"`
	Numerator   int        `json:"numerator"`
	Denominator int        `json:"denominator"`
	Evidence    []Evidence `json:"evidence"`
}
type FirstCIMetric struct {
	Successful int        `json:"successful"`
	Eligible   int        `json:"eligible"`
	Rate       float64    `json:"rate"`
	Evidence   []Evidence `json:"evidence"`
}
type CycleMetric struct {
	Completed     int        `json:"completed"`
	MedianSeconds float64    `json:"median_seconds"`
	Evidence      []Evidence `json:"evidence"`
}
type Evidence struct {
	DeliveryID  string    `json:"delivery_id"`
	EventType   string    `json:"event_type"`
	ReceivedAt  time.Time `json:"received_at"`
	PRNumber    *int64    `json:"pr_number,omitempty"`
	IssueNumber *int64    `json:"issue_number,omitempty"`
}
type Suggestion struct {
	Key         string     `json:"key"`
	Message     string     `json:"message"`
	Occurrences int        `json:"occurrences"`
	Evidence    []Evidence `json:"evidence"`
}
