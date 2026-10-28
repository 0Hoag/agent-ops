package agentrun

import "time"

type CreateInput struct {
	RepositoryID      string    `json:"repository_id" binding:"required"`
	Wave              string    `json:"wave" binding:"required"`
	Worker            string    `json:"worker" binding:"required"`
	Model             string    `json:"model" binding:"required"`
	IssueNumbers      []int64   `json:"issue_numbers"`
	PRNumber          *int64    `json:"pr_number"`
	InputTokens       int64     `json:"input_tokens"`
	CachedInputTokens int64     `json:"cached_input_tokens"`
	CacheWriteTokens  int64     `json:"cache_write_tokens"`
	OutputTokens      int64     `json:"output_tokens"`
	ReasoningTokens   int64     `json:"reasoning_tokens"`
	StartedAt         time.Time `json:"started_at" binding:"required"`
	EndedAt           time.Time `json:"ended_at" binding:"required"`
	Harness           string    `json:"harness,omitempty"`
	SessionID         string    `json:"session_id,omitempty"`
}

type Filter struct {
	RepositoryID, Wave string
	IssueNumber        int64
}
type Aggregate struct {
	Wave              string   `json:"wave"`
	Worker            string   `json:"worker"`
	IssueNumber       *int64   `json:"issue_number,omitempty"`
	Runs              int64    `json:"runs"`
	InputTokens       int64    `json:"input_tokens"`
	CachedInputTokens int64    `json:"cached_input_tokens"`
	CacheWriteTokens  int64    `json:"cache_write_tokens"`
	OutputTokens      int64    `json:"output_tokens"`
	ReasoningTokens   int64    `json:"reasoning_tokens"`
	CostUSD           *float64 `json:"cost_usd,omitempty"`
	CostVND           *float64 `json:"cost_vnd,omitempty"`
	EquivalentCostUSD *float64 `json:"equivalent_cost_usd,omitempty"`
	BilledCostUSD     *float64 `json:"billed_cost_usd,omitempty"`
}

type CreatedResponse struct {
	Status int      `json:"status"`
	Data   AgentRun `json:"data"`
}

type ListResponse struct {
	Status int        `json:"status"`
	Data   []AgentRun `json:"data"`
}

type AggregateResponse struct {
	Status int         `json:"status"`
	Data   []Aggregate `json:"data"`
}
