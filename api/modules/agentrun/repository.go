package agentrun

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Create(ctx context.Context, run *AgentRun) error {
	if err := r.db.WithContext(ctx).Create(run).Error; err != nil {
		return fmt.Errorf("create agent run: %w", err)
	}
	return nil
}
func (r *Repository) List(ctx context.Context, f Filter) ([]AgentRun, error) {
	q := r.db.WithContext(ctx).Model(&AgentRun{})
	if f.RepositoryID != "" {
		q = q.Where("repository_id = ?", f.RepositoryID)
	}
	if f.Wave != "" {
		q = q.Where("wave = ?", f.Wave)
	}
	if f.IssueNumber != 0 {
		q = q.Where("issue_numbers @> ?::jsonb", fmt.Sprintf("[%d]", f.IssueNumber))
	}
	var out []AgentRun
	if err := q.Order("started_at DESC").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list agent runs: %w", err)
	}
	return out, nil
}
func (r *Repository) Aggregate(ctx context.Context, f Filter, prices map[string]Price) ([]Aggregate, error) {
	runs, err := r.List(ctx, f)
	if err != nil {
		return nil, err
	}
	groups := map[string]*Aggregate{}
	order := []string{}
	for _, run := range runs {
		var issues []int64
		_ = jsonUnmarshal(run.IssueNumbers, &issues)
		if len(issues) == 0 {
			issues = []int64{0}
		}
		for _, issue := range issues {
			key := run.Wave + "\x00" + run.Worker + fmt.Sprint(issue)
			a := groups[key]
			if a == nil {
				a = &Aggregate{Wave: run.Wave, Worker: run.Worker}
				if issue != 0 {
					x := issue
					a.IssueNumber = &x
				}
				groups[key] = a
				order = append(order, key)
			}
			a.Runs++
			a.InputTokens += run.InputTokens
			a.CachedInputTokens += run.CachedInputTokens
			a.CacheWriteTokens += run.CacheWriteTokens
			a.OutputTokens += run.OutputTokens
			a.ReasoningTokens += run.ReasoningTokens
			if p, ok := prices[run.Model]; ok {
				v := priceCost(run, p)
				if a.CostUSD == nil {
					x := 0.0
					a.CostUSD = &x
				}
				*a.CostUSD += v
			} else {
				a.CostUSD = nil
			}
		}
	}
	out := make([]Aggregate, 0, len(order))
	for _, k := range order {
		out = append(out, *groups[k])
	}
	return out, nil
}

type Price struct {
	InputPerMillion       float64 `json:"input_per_million"`
	CachedInputPerMillion float64 `json:"cached_input_per_million"`
	CacheWritePerMillion  float64 `json:"cache_write_per_million"`
	OutputPerMillion      float64 `json:"output_per_million"`
	ReasoningPerMillion   float64 `json:"reasoning_per_million"`
}

func priceCost(r AgentRun, p Price) float64 {
	return float64(r.InputTokens)*p.InputPerMillion/1e6 + float64(r.CachedInputTokens)*p.CachedInputPerMillion/1e6 + float64(r.CacheWriteTokens)*p.CacheWritePerMillion/1e6 + float64(r.OutputTokens)*p.OutputPerMillion/1e6 + float64(r.ReasoningTokens)*p.ReasoningPerMillion/1e6
}
func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
