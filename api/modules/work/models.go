package work

import (
	"context"
	"time"
)

type contextKey string

const organizationKey contextKey = "work_organization_id"

func WithOrganization(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, organizationKey, id)
}

type Item struct {
	Kind         string    `json:"kind"`
	Number       int64     `json:"number"`
	Title        string    `json:"title"`
	URL          string    `json:"url"`
	Author       string    `json:"author"`
	State        string    `json:"state"`
	CIStatus     string    `json:"ci_status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	RunsCount    int64     `json:"runs_count"`
	CostUSD      *float64  `json:"cost_usd"`
	Tokens       int64     `json:"tokens"`
	LastActivity time.Time `json:"last_activity"`
}

type Run struct {
	ID        string    `json:"id"`
	Model     string    `json:"model"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	CostUSD   *float64  `json:"cost_usd"`
	Tokens    int64     `json:"tokens"`
	CacheHit  bool      `json:"cache_hit"`
}

type Detail struct {
	Item Item  `json:"item"`
	Runs []Run `json:"runs"`
}
type Meta struct {
	Page    int   `json:"page"`
	PerPage int   `json:"per_page"`
	Total   int64 `json:"total"`
}
type ListResponse struct {
	Data []Item `json:"data"`
	Meta Meta   `json:"meta"`
}
