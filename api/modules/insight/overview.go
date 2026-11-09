package insight

import (
	"context"
	"fmt"
	"time"
)

type OverviewFilter struct {
	RepositoryID string
	From, To     time.Time
}
type OverviewResponse struct {
	Current   OverviewPeriod      `json:"current"`
	Previous  OverviewPeriod      `json:"previous"`
	Attention []OverviewAttention `json:"attention"`
}
type OverviewPeriod struct {
	From               string   `json:"from"`
	To                 string   `json:"to"`
	SpendUSD           *float64 `json:"spend_usd"`
	InputTokens        int64    `json:"input_tokens"`
	OutputTokens       int64    `json:"output_tokens"`
	CacheReadTokens    int64    `json:"cache_read_tokens"`
	CacheWriteTokens   int64    `json:"cache_write_tokens"`
	CacheHitRate       float64  `json:"cache_hit_rate"`
	CacheSavingsUSD    *float64 `json:"cache_savings_usd"`
	Runs               int      `json:"runs"`
	MergedPRs          int      `json:"merged_prs"`
	CostPerMergedPR    *float64 `json:"cost_per_merged_pr"`
	ReworkRate         float64  `json:"rework_rate"`
	CIFirstPassRate    float64  `json:"ci_first_pass_rate"`
	ModelsWithoutPrice []string `json:"models_without_price"`
}
type OverviewAttention struct {
	Kind         string `json:"kind"`
	RepositoryID string `json:"repository_id,omitempty"`
	PRNumber     *int64 `json:"pr_number,omitempty"`
	RunID        string `json:"run_id,omitempty"`
	Model        string `json:"model,omitempty"`
	Message      string `json:"message"`
}
type overviewRun struct {
	AnalyticsRun
	EndedAt  time.Time
	PRNumber *int64
}

func (r *Repository) Overview(ctx context.Context, f OverviewFilter) (OverviewResponse, error) {
	if f.From.IsZero() || f.To.Before(f.From) || f.To.Sub(f.From) > 366*24*time.Hour {
		return OverviewResponse{}, fmt.Errorf("invalid overview date range")
	}
	length := f.To.Sub(f.From) + 24*time.Hour
	cur, err := r.overviewPeriod(ctx, f.RepositoryID, f.From, f.To.Add(24*time.Hour))
	if err != nil {
		return OverviewResponse{}, err
	}
	prevFrom, prevTo := f.From.Add(-length), f.From
	prev, err := r.overviewPeriod(ctx, f.RepositoryID, prevFrom, prevTo)
	if err != nil {
		return OverviewResponse{}, err
	}
	attention, err := r.overviewAttention(ctx, f.RepositoryID, f.To.Add(24*time.Hour))
	if err != nil {
		return OverviewResponse{}, err
	}
	return OverviewResponse{Current: cur, Previous: prev, Attention: attention}, nil
}

func (r *Repository) overviewPeriod(ctx context.Context, repo string, from, to time.Time) (OverviewPeriod, error) {
	q := r.db.WithContext(ctx).Table("agent_runs ar").Joins("JOIN repositories repo ON repo.id = ar.repository_id").Where("repo.organization_id = ? AND (ar.started_at >= ? AND ar.started_at < ? OR EXISTS (SELECT 1 FROM agent_run_steps ts WHERE ts.run_id = ar.id AND ts.started_at >= ? AND ts.started_at < ?))", organizationFromContext(ctx), from, to, from, to)
	if repo != "" {
		q = q.Where("ar.repository_id = ?", repo)
	}
	var runs []overviewRun
	if err := q.Select("ar.id, ar.repository_id, ar.wave, ar.model, ar.billing_type, ar.harness, ar.issue_numbers, ar.input_tokens, ar.cached_input_tokens, ar.cache_write_tokens, ar.output_tokens, ar.reasoning_tokens, ar.started_at, ar.ended_at, ar.pr_number").Order("ar.started_at, ar.id").Find(&runs).Error; err != nil {
		return OverviewPeriod{}, fmt.Errorf("list overview runs: %w", err)
	}
	prices, err := r.loadPrices(ctx, organizationFromContext(ctx))
	if err != nil {
		return OverviewPeriod{}, err
	}
	out := OverviewPeriod{From: from.UTC().Format("2006-01-02"), To: to.Add(-24 * time.Hour).UTC().Format("2006-01-02"), ModelsWithoutPrice: []string{}}
	seenMissing := map[string]bool{}
	denom := int64(0)
	cacheSavings := 0.0
	hasSavings := false
	for _, run := range runs {
		steps, err := r.usageSteps(ctx, run.ID)
		if err != nil {
			return OverviewPeriod{}, err
		}
		if compatibleTimeline(run.AnalyticsRun, steps) {
			for _, step := range steps {
				if step.StartedAt == nil || step.StartedAt.Before(from) || !step.StartedAt.Before(to) {
					continue
				}
				r.addOverviewUsage(&out, run, stepRun(run.AnalyticsRun, step), *step.StartedAt, prices, &seenMissing, &denom, &cacheSavings, &hasSavings)
			}
			continue
		}
		r.addOverviewUsage(&out, run, run.AnalyticsRun, run.StartedAt, prices, &seenMissing, &denom, &cacheSavings, &hasSavings)
	}
	if denom > 0 {
		out.CacheHitRate = float64(out.CacheReadTokens) / float64(denom)
	}
	if hasSavings {
		out.CacheSavingsUSD = &cacheSavings
	}
	var merged int64
	mq := r.db.WithContext(ctx).Table("pull_requests pr").Joins("JOIN repositories repo ON repo.id = pr.repository_id").Where("repo.organization_id = ? AND pr.merged_at >= ? AND pr.merged_at < ?", organizationFromContext(ctx), from, to)
	if repo != "" {
		mq = mq.Where("pr.repository_id = ?", repo)
	}
	if err := mq.Count(&merged).Error; err != nil {
		return OverviewPeriod{}, fmt.Errorf("count merged overview pull requests: %w", err)
	}
	out.MergedPRs = int(merged)
	if out.MergedPRs > 0 && out.SpendUSD != nil {
		v := *out.SpendUSD / float64(out.MergedPRs)
		out.CostPerMergedPR = &v
	}
	deliveryTo := to.Add(-24 * time.Hour)
	delivery, err := r.Delivery(ctx, DeliveryFilter{RepositoryID: repo, From: &from, To: &deliveryTo})
	if err != nil {
		return OverviewPeriod{}, err
	}
	out.ReworkRate = delivery.SendBack.Rate
	out.CIFirstPassRate = delivery.FirstCI.Rate
	return out, nil
}

func (r *Repository) addOverviewUsage(out *OverviewPeriod, row overviewRun, run AnalyticsRun, at time.Time, prices []priceRow, seenMissing *map[string]bool, denom *int64, cacheSavings *float64, hasSavings *bool) {
	out.Runs++
	out.InputTokens += run.InputTokens
	out.OutputTokens += run.OutputTokens
	out.CacheReadTokens += run.CachedInputTokens
	out.CacheWriteTokens += run.CacheWriteTokens
	*denom += run.InputTokens + run.CachedInputTokens
	p, ok := priceAt(prices, run.Model, at)
	if !ok {
		if !(*seenMissing)[run.Model] {
			out.ModelsWithoutPrice = append(out.ModelsWithoutPrice, run.Model)
			(*seenMissing)[run.Model] = true
		}
		return
	}
	c := cost(run, p)
	out.SpendUSD = add(out.SpendUSD, c)
	baseline := float64(run.CachedInputTokens) * p.InputPerMillion / 1e6
	actual := float64(run.CachedInputTokens) * p.CachedInputPerMillion / 1e6
	*cacheSavings += baseline - actual
	*hasSavings = true
}

func (r *Repository) overviewAttention(ctx context.Context, repo string, now time.Time) ([]OverviewAttention, error) {
	org := organizationFromContext(ctx)
	out := []OverviewAttention{}
	q := r.db.WithContext(ctx).Table("pull_requests pr").Joins("JOIN repositories repo ON repo.id = pr.repository_id").Where("repo.organization_id = ? AND pr.state = ? AND pr.updated_at <= ?", org, "open", now.Add(-72*time.Hour))
	if repo != "" {
		q = q.Where("pr.repository_id = ?", repo)
	}
	var prs []struct {
		RepositoryID string
		Number       int64
	}
	if err := q.Select("pr.repository_id, pr.number").Find(&prs).Error; err != nil {
		return nil, fmt.Errorf("list stale overview pull requests: %w", err)
	}
	for _, p := range prs {
		n := p.Number
		out = append(out, OverviewAttention{Kind: "stale_pr", RepositoryID: p.RepositoryID, PRNumber: &n, Message: "open pull request has had no activity for at least 3 days"})
	}
	q = r.db.WithContext(ctx).Table("agent_runs ar").Joins("JOIN repositories repo ON repo.id = ar.repository_id").Where("repo.organization_id = ? AND ar.pr_number IS NULL AND ar.started_at <= ?", org, now.Add(-24*time.Hour))
	if repo != "" {
		q = q.Where("ar.repository_id = ?", repo)
	}
	var rs []struct {
		ID    string
		Model string
	}
	if err := q.Select("ar.id, ar.model").Find(&rs).Error; err != nil {
		return nil, fmt.Errorf("list unlinked overview runs: %w", err)
	}
	for _, x := range rs {
		out = append(out, OverviewAttention{Kind: "unlinked_run", RunID: x.ID, Model: x.Model, Message: "agent session has no linked pull request after 24 hours"})
	}
	var failing []struct {
		RepositoryID string
		Number       int64
	}
	q = r.db.WithContext(ctx).Table("pipeline_runs p").Joins("JOIN repositories repo ON repo.id = p.repository_id").Joins("JOIN pull_requests pr ON pr.repository_id=p.repository_id AND pr.number=p.pr_number").Where("repo.organization_id = ? AND p.conclusion IN ? AND p.updated_at <= ? AND pr.state = ?", org, []string{"failure", "failed"}, now.Add(-24*time.Hour), "open")
	if repo != "" {
		q = q.Where("p.repository_id = ?", repo)
	}
	if err := q.Select("p.repository_id, p.pr_number AS number").Find(&failing).Error; err != nil {
		return nil, fmt.Errorf("list failing overview pipelines: %w", err)
	}
	for _, p := range failing {
		n := p.Number
		out = append(out, OverviewAttention{Kind: "failing_ci", RepositoryID: p.RepositoryID, PRNumber: &n, Message: "open pull request has failing CI for more than 24 hours"})
	}
	return out, nil
}
