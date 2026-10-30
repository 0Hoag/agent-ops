package budget

import (
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
)

type Budget struct {
	ID             store.UUID  `json:"id" gorm:"type:uuid;primaryKey"`
	OrganizationID store.UUID  `json:"organization_id" gorm:"type:uuid;not null"`
	RepositoryID   *store.UUID `json:"repository_id,omitempty" gorm:"type:uuid"`
	Wave           string      `json:"wave,omitempty"`
	TokenLimit     *int64      `json:"token_limit,omitempty"`
	USDLimit       *float64    `json:"usd_limit,omitempty"`
	StartsAt       time.Time   `json:"starts_at"`
	EndsAt         time.Time   `json:"ends_at"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

func (Budget) TableName() string { return "budgets" }

type Usage struct {
	Tokens int64
	USD    float64
}
type Threshold struct{ Token80, Token100, USD80, USD100 bool }

func Evaluate(limit Budget, usage Usage) Threshold {
	var out Threshold
	if limit.TokenLimit != nil {
		out.Token80 = float64(usage.Tokens) >= float64(*limit.TokenLimit)*.8
		out.Token100 = usage.Tokens >= *limit.TokenLimit
	}
	if limit.USDLimit != nil {
		out.USD80 = usage.USD >= *limit.USDLimit*.8
		out.USD100 = usage.USD >= *limit.USDLimit
	}
	return out
}
