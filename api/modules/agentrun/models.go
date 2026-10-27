package agentrun

import (
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
)

type AgentRun struct {
	ID                store.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	RepositoryID      store.UUID  `gorm:"type:uuid;not null;index" json:"repository_id"`
	Wave              string      `gorm:"not null;index" json:"wave"`
	Worker            string      `gorm:"not null;index" json:"worker"`
	Model             string      `gorm:"not null" json:"model"`
	IssueNumbers      store.JSONB `gorm:"type:jsonb;not null" json:"issue_numbers" swaggertype:"array,integer"`
	PRNumber          *int64      `json:"pr_number,omitempty"`
	InputTokens       int64       `json:"input_tokens"`
	CachedInputTokens int64       `json:"cached_input_tokens"`
	CacheWriteTokens  int64       `json:"cache_write_tokens"`
	OutputTokens      int64       `json:"output_tokens"`
	ReasoningTokens   int64       `json:"reasoning_tokens"`
	CostUSD           *float64    `json:"cost_usd,omitempty"`
	StartedAt         time.Time   `json:"started_at"`
	EndedAt           time.Time   `json:"ended_at"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
	Harness           *string     `json:"harness,omitempty"`
	SessionID         *string     `json:"session_id,omitempty"`
}
