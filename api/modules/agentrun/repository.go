package agentrun

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Create(ctx context.Context, run *AgentRun) error {
	q := r.db.WithContext(ctx)
	if run.Harness != nil && run.SessionID != nil && *run.Harness != "" && *run.SessionID != "" {
		q = q.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "repository_id"}, {Name: "harness"}, {Name: "session_id"}}, TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "harness <> '' AND session_id <> ''"}}}, DoUpdates: clause.AssignmentColumns([]string{"wave", "worker", "model", "issue_numbers", "pr_number", "input_tokens", "cached_input_tokens", "cache_write_tokens", "output_tokens", "reasoning_tokens", "started_at", "ended_at", "updated_at"})})
	}
	if err := q.Create(run).Error; err != nil {
		return fmt.Errorf("create agent run: %w", err)
	}
	return nil
}

func (r *Repository) IsConfiguredRepository(ctx context.Context, id string, names []string) (bool, error) {
	if len(names) == 0 {
		return false, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Table("repositories").Where("id = ? AND full_name IN ?", id, names).Count(&count).Error
	return count == 1, err
}
func (r *Repository) List(ctx context.Context, f Filter) ([]AgentRun, error) {
	q := r.listQuery(ctx, f)
	var out []AgentRun
	if err := q.Order("started_at DESC").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list agent runs: %w", err)
	}
	return out, nil
}
func (r *Repository) listQuery(ctx context.Context, f Filter) *gorm.DB {
	q := r.db.WithContext(ctx).Model(&AgentRun{})
	if organization := organizationFromContext(ctx); organization != "" {
		q = q.Joins("JOIN repositories ON repositories.id = agent_runs.repository_id").Where("repositories.organization_id = ?", organization)
	}
	if f.RepositoryID != "" {
		q = q.Where("repository_id = ?", f.RepositoryID)
	}
	if f.Wave != "" {
		q = q.Where("wave = ?", f.Wave)
	}
	if f.IssueNumber != 0 {
		q = q.Where("issue_numbers @> ?::jsonb", fmt.Sprintf("[%d]", f.IssueNumber))
	}
	if f.Harness != "" {
		q = q.Where("harness = ?", f.Harness)
	}
	if f.Model != "" {
		q = q.Where("model = ?", f.Model)
	}
	if f.PRNumber != 0 {
		q = q.Where("pr_number = ?", f.PRNumber)
	}
	if f.StartedAfter != nil {
		q = q.Where("started_at >= ?", *f.StartedAfter)
	}
	if f.StartedBefore != nil {
		q = q.Where("started_at <= ?", *f.StartedBefore)
	}
	return q
}
func (r *Repository) ListPage(ctx context.Context, f Filter, page, perPage int) ([]AgentRun, int64, error) {
	q := r.listQuery(ctx, f)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count agent runs: %w", err)
	}
	var out []AgentRun
	if err := q.Order("agent_runs.started_at DESC, agent_runs.id DESC").Offset((page - 1) * perPage).Limit(perPage).Find(&out).Error; err != nil {
		return nil, 0, fmt.Errorf("list agent runs: %w", err)
	}
	if err := r.attachRelations(ctx, out); err != nil {
		return nil, 0, fmt.Errorf("load run relations: %w", err)
	}
	return out, total, nil
}

func (r *Repository) attachRelations(ctx context.Context, runs []AgentRun) error {
	for i := range runs {
		var nums []int64
		_ = json.Unmarshal(runs[i].IssueNumbers, &nums)
		rel := &RunRelations{Issues: []RunIssue{}, PullRequests: []RunPullRequest{}, CI: []RunCI{}}
		if len(nums) > 0 {
			q := r.db.WithContext(ctx).Table("issues").Select("repository_id,number,title,state,status").Where("repository_id = ?", runs[i].RepositoryID)
			q = q.Where("number IN ?", nums)
			if err := q.Order("number ASC").Find(&rel.Issues).Error; err != nil {
				return err
			}
		}
		if runs[i].PRNumber != nil {
			if err := r.db.WithContext(ctx).Table("pull_requests").Select("repository_id,number,title,state,head_sha").Where("repository_id = ? AND number = ?", runs[i].RepositoryID, *runs[i].PRNumber).Find(&rel.PullRequests).Error; err != nil {
				return err
			}
			if err := r.db.WithContext(ctx).Table("pipeline_runs").Select("repository_id,external_id,pr_number,sha,status,conclusion").Where("repository_id = ? AND pr_number = ?", runs[i].RepositoryID, *runs[i].PRNumber).Order("started_at DESC NULLS LAST, external_id ASC").Find(&rel.CI).Error; err != nil {
				return err
			}
		}
		runs[i].Relations = rel
	}
	return nil
}
func (r *Repository) Aggregate(ctx context.Context, f Filter, prices map[string]Price) ([]Aggregate, error) {
	runs, err := r.List(ctx, f)
	if err != nil {
		return nil, err
	}
	groups := map[string]*Aggregate{}
	unknownCosts := map[string]struct{}{}
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
				if _, unknown := unknownCosts[key]; !unknown {
					v := priceCost(run, p)
					if a.CostUSD == nil {
						x := 0.0
						a.CostUSD = &x
					}
					*a.CostUSD += v
				}
			} else {
				a.CostUSD = nil
				unknownCosts[key] = struct{}{}
			}
		}
	}
	out := make([]Aggregate, 0, len(order))
	for _, k := range order {
		out = append(out, *groups[k])
	}
	return out, nil
}

func (r *Repository) AggregateHistorical(ctx context.Context, f Filter, fallback map[string]Price) ([]Aggregate, error) {
	runs, err := r.List(ctx, f)
	if err != nil {
		return nil, err
	}
	groups := map[string]*Aggregate{}
	unknown := map[string]bool{}
	for _, run := range runs {
		var issues []int64
		_ = jsonUnmarshal(run.IssueNumbers, &issues)
		if len(issues) == 0 {
			issues = []int64{0}
		}
		p, priceErr := r.priceAt(ctx, organizationFromContext(ctx), run.Model, run.StartedAt)
		if priceErr != nil {
			return nil, fmt.Errorf("resolve historical price: %w", priceErr)
		}
		if p == nil {
			if fp, ok := fallback[run.Model]; ok {
				p = &fp
			}
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
			}
			a.Runs++
			a.InputTokens += run.InputTokens
			a.CachedInputTokens += run.CachedInputTokens
			a.CacheWriteTokens += run.CacheWriteTokens
			a.OutputTokens += run.OutputTokens
			a.ReasoningTokens += run.ReasoningTokens
			if p == nil {
				unknown[key] = true
				a.CostUSD = nil
			} else if !unknown[key] {
				v := priceCost(run, *p)
				if a.CostUSD == nil {
					x := 0.0
					a.CostUSD = &x
				}
				*a.CostUSD += v
				x := v
				a.EquivalentCostUSD = addCost(a.EquivalentCostUSD, x)
				if run.BillingType == "subscription" {
					a.BilledCostUSD = addCost(a.BilledCostUSD, 0)
				} else {
					a.BilledCostUSD = addCost(a.BilledCostUSD, x)
				}
			}
		}
	}
	order := make([]string, 0, len(groups))
	for k := range groups {
		order = append(order, k)
	}
	out := make([]Aggregate, 0, len(order))
	for _, k := range order {
		out = append(out, *groups[k])
	}
	return out, nil
}

func addCost(current *float64, value float64) *float64 {
	if current == nil {
		current = new(float64)
	}
	*current += value
	return current
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
