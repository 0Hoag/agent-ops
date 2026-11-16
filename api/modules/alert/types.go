package alert

import (
	"time"

	"github.com/0Hoag/agent-ops/api/modules/budget"
)

const (
	RuleLowCache = "low_cache_hit"
	RuleSilent   = "silent_worker"
	RuleRedCI    = "red_main_ci"
	RuleStalePR  = "stale_open_pr"
)

type Thresholds struct {
	MinCacheHit  float64
	SilentAfter  time.Duration
	PRStaleAfter time.Duration
}

type Observation struct {
	OrganizationID string
	Repository     string
	Runs           []RunObservation
	Pipelines      []PipelineObservation
	PullRequests   []PullRequestObservation
	Budgets        []BudgetObservation
}

type BudgetObservation struct {
	ID     string
	Tokens int64
	USD    *float64
	Limit  budget.Budget
}

type RunObservation struct {
	Worker            string
	StartedAt         time.Time
	LastActivityAt    time.Time
	InputTokens       int64
	CachedInputTokens int64
	Active            bool
}

type PipelineObservation struct {
	ID         string
	Branch     string
	Status     string
	Conclusion string
}

type PullRequestObservation struct {
	Number    int64
	State     string
	Merged    bool
	UpdatedAt time.Time
}

type Alert struct {
	Key            string
	OrganizationID string
	Repository     string
	Rule           string
	Subject        string
	DetectedAt     time.Time
	Message        string
}
