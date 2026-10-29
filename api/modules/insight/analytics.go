package insight

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
)

var validAnalyticsGroups = map[string]bool{"harness": true, "model": true, "wave": true, "area": true}

type AnalyticsFilter struct {
	RepositoryID, GroupBy, Period string
	From, To                      *time.Time
}
type AnalyticsRun struct {
	ID, RepositoryID, Wave, Model, BillingType                                      string
	Harness                                                                         *string
	IssueNumbers                                                                    store.JSONB
	InputTokens, CachedInputTokens, CacheWriteTokens, OutputTokens, ReasoningTokens int64
	StartedAt                                                                       time.Time
}
type analyticsIssue struct {
	RepositoryID string
	Number       int64
	Labels       store.JSONB
}

type AnalyticsResponse struct {
	GroupBy           string            `json:"group_by"`
	Period            string            `json:"period"`
	From              string            `json:"from"`
	To                string            `json:"to"`
	BucketTimestamp   string            `json:"bucket_timestamp"`
	WeekStarts        string            `json:"week_starts"`
	CacheDenominator  string            `json:"cache_denominator"`
	UnknownPriceCount int               `json:"unknown_price_count"`
	Buckets           []AnalyticsBucket `json:"buckets"`
}
type AnalyticsBucket struct {
	Start             string   `json:"start"`
	Key               string   `json:"key"`
	Runs              int      `json:"runs"`
	InputTokens       int64    `json:"input_tokens"`
	CachedInputTokens int64    `json:"cached_input_tokens"`
	CacheWriteTokens  int64    `json:"cache_write_tokens"`
	OutputTokens      int64    `json:"output_tokens"`
	ReasoningTokens   int64    `json:"reasoning_tokens"`
	CacheHitRate      float64  `json:"cache_hit_rate"`
	ActualCostUSD     *float64 `json:"actual_cost_usd"`
	EquivalentCostUSD *float64 `json:"equivalent_cost_usd"`
	UnknownPriceCount int      `json:"unknown_price_count"`
}

func (r *Repository) Analytics(ctx context.Context, f AnalyticsFilter) (AnalyticsResponse, error) {
	if !validAnalyticsGroups[f.GroupBy] || (f.Period != "daily" && f.Period != "weekly") {
		return AnalyticsResponse{}, fmt.Errorf("invalid analytics filter")
	}
	if f.From == nil || f.To == nil || f.To.Before(*f.From) || f.To.Sub(*f.From) > 366*24*time.Hour {
		return AnalyticsResponse{}, fmt.Errorf("invalid analytics date range")
	}
	q := r.db.WithContext(ctx).Table("agent_runs ar").Joins("JOIN repositories repo ON repo.id = ar.repository_id").Where("repo.organization_id = ? AND ar.started_at >= ? AND ar.started_at < ?", organizationFromContext(ctx), *f.From, f.To.Add(24*time.Hour))
	if f.RepositoryID != "" {
		q = q.Where("ar.repository_id = ?", f.RepositoryID)
	}
	var runs []AnalyticsRun
	if err := q.Select("ar.id, ar.repository_id, ar.wave, ar.model, ar.billing_type, ar.harness, ar.issue_numbers, ar.input_tokens, ar.cached_input_tokens, ar.cache_write_tokens, ar.output_tokens, ar.reasoning_tokens, ar.started_at").Order("ar.started_at ASC, ar.id ASC").Find(&runs).Error; err != nil {
		return AnalyticsResponse{}, fmt.Errorf("list analytics runs: %w", err)
	}
	prices, err := r.loadPrices(ctx, organizationFromContext(ctx))
	if err != nil {
		return AnalyticsResponse{}, err
	}
	result := AnalyticsResponse{GroupBy: f.GroupBy, Period: f.Period, From: f.From.UTC().Format("2006-01-02"), To: f.To.UTC().Format("2006-01-02"), BucketTimestamp: "UTC interval start", WeekStarts: "Monday", CacheDenominator: "input_tokens + cached_input_tokens", Buckets: []AnalyticsBucket{}}
	for _, run := range runs {
		area := r.area(ctx, run)
		key := area
		switch f.GroupBy {
		case "harness":
			key = value(run.Harness)
		case "model":
			key = run.Model
		case "wave":
			key = run.Wave
		}
		start := run.StartedAt.UTC().Truncate(24 * time.Hour)
		if f.Period == "weekly" {
			start = monday(start)
		}
		idx := -1
		for i := range result.Buckets {
			if result.Buckets[i].Start == start.Format(time.RFC3339) && result.Buckets[i].Key == key {
				idx = i
				break
			}
		}
		if idx < 0 {
			result.Buckets = append(result.Buckets, AnalyticsBucket{Start: start.Format(time.RFC3339), Key: key})
			idx = len(result.Buckets) - 1
		}
		b := &result.Buckets[idx]
		b.Runs++
		b.InputTokens += run.InputTokens
		b.CachedInputTokens += run.CachedInputTokens
		b.CacheWriteTokens += run.CacheWriteTokens
		b.OutputTokens += run.OutputTokens
		b.ReasoningTokens += run.ReasoningTokens
		if p, ok := priceAt(prices, run.Model, run.StartedAt); ok {
			cost := cost(run, p)
			b.EquivalentCostUSD = add(b.EquivalentCostUSD, cost)
			if run.BillingType != "subscription" {
				b.ActualCostUSD = add(b.ActualCostUSD, cost)
			}
		} else {
			b.UnknownPriceCount++
			result.UnknownPriceCount++
		}
		denom := b.InputTokens + b.CachedInputTokens
		if denom > 0 {
			b.CacheHitRate = float64(b.CachedInputTokens) / float64(denom)
		}
	}
	return result, nil
}

func (r *Repository) loadPrices(ctx context.Context, org string) ([]priceRow, error) {
	var p []priceRow
	if err := r.db.WithContext(ctx).Table("agent_run_price_versions").Where("organization_id = ?", org).Order("model, effective_date ASC").Find(&p).Error; err != nil {
		return nil, fmt.Errorf("load analytics prices: %w", err)
	}
	return p, nil
}

type priceRow struct {
	Model                                                                                               string
	EffectiveDate                                                                                       time.Time
	InputPerMillion, CachedInputPerMillion, CacheWritePerMillion, OutputPerMillion, ReasoningPerMillion float64
}

func priceAt(ps []priceRow, model string, at time.Time) (priceRow, bool) {
	var out priceRow
	ok := false
	for _, p := range ps {
		if p.Model == model && !p.EffectiveDate.After(at.UTC()) {
			out = p
			ok = true
		}
	}
	return out, ok
}
func cost(r AnalyticsRun, p priceRow) float64 {
	return float64(r.InputTokens)*p.InputPerMillion/1e6 + float64(r.CachedInputTokens)*p.CachedInputPerMillion/1e6 + float64(r.CacheWriteTokens)*p.CacheWritePerMillion/1e6 + float64(r.OutputTokens)*p.OutputPerMillion/1e6 + float64(r.ReasoningTokens)*p.ReasoningPerMillion/1e6
}
func add(p *float64, v float64) *float64 {
	if p == nil {
		p = new(float64)
	}
	*p += v
	return p
}
func value(v *string) string {
	if v == nil || strings.TrimSpace(*v) == "" {
		return "unknown"
	}
	return *v
}
func monday(t time.Time) time.Time { d := (int(t.Weekday()) + 6) % 7; return t.AddDate(0, 0, -d) }
func (r *Repository) area(ctx context.Context, run AnalyticsRun) string {
	var nums []int64
	_ = json.Unmarshal(run.IssueNumbers, &nums)
	if len(nums) != 1 {
		return "unknown"
	}
	var i analyticsIssue
	if r.db.WithContext(ctx).Table("issues").Select("repository_id,number,labels").Where("repository_id = ? AND number = ?", run.RepositoryID, nums[0]).First(&i).Error != nil {
		return "unknown"
	}
	var labels []struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(i.Labels, &labels) != nil {
		return "unknown"
	}
	var found string
	for _, l := range labels {
		if strings.HasPrefix(l.Name, "area:") {
			a := strings.TrimPrefix(l.Name, "area:")
			if found != "" && found != a {
				return "unknown"
			}
			found = a
		}
	}
	if found == "" {
		return "unknown"
	}
	return found
}
