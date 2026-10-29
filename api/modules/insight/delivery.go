package insight

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
)

type deliveryPR struct {
	RepositoryID string
	Number       int64
	MergedAt     *time.Time
	CreatedAt    time.Time
	Closes       store.JSONB
}
type deliveryIssue struct {
	RepositoryID string
	Number       int64
	State        string
	CreatedAt    time.Time
}
type deliveryRun struct {
	ID                                                                              string
	RepositoryID                                                                    string
	PRNumber                                                                        *int64
	IssueNumbers                                                                    store.JSONB
	Model                                                                           string
	Harness                                                                         *string
	BillingType                                                                     string
	InputTokens, CachedInputTokens, CacheWriteTokens, OutputTokens, ReasoningTokens int64
	StartedAt                                                                       time.Time
}

func (r *Repository) Delivery(ctx context.Context, f DeliveryFilter) (DeliveryResponse, error) {
	if f.From == nil || f.To == nil || f.To.Before(*f.From) || f.To.Sub(*f.From) > 366*24*time.Hour {
		return DeliveryResponse{}, fmt.Errorf("invalid delivery date range")
	}
	org := organizationFromContext(ctx)
	prq := r.db.WithContext(ctx).Table("pull_requests pr").Joins("JOIN repositories repo ON repo.id = pr.repository_id").Where("repo.organization_id = ? AND pr.created_at >= ? AND pr.created_at < ?", org, *f.From, f.To.Add(24*time.Hour))
	iq := r.db.WithContext(ctx).Table("issues i").Joins("JOIN repositories repo ON repo.id = i.repository_id").Where("repo.organization_id = ? AND i.created_at >= ? AND i.created_at < ?", org, *f.From, f.To.Add(24*time.Hour))
	rq := r.db.WithContext(ctx).Table("agent_runs ar").Joins("JOIN repositories repo ON repo.id = ar.repository_id").Where("repo.organization_id = ? AND ar.started_at >= ? AND ar.started_at < ?", org, *f.From, f.To.Add(24*time.Hour))
	if f.RepositoryID != "" {
		prq = prq.Where("pr.repository_id = ?", f.RepositoryID)
		iq = iq.Where("i.repository_id = ?", f.RepositoryID)
		rq = rq.Where("ar.repository_id = ?", f.RepositoryID)
	}
	var prs []deliveryPR
	var issues []deliveryIssue
	var runs []deliveryRun
	var retained []store.WebhookEvent
	if err := prq.Select("pr.repository_id, pr.number, pr.merged_at, pr.created_at, pr.closes").Order("pr.repository_id, pr.number").Find(&prs).Error; err != nil {
		return DeliveryResponse{}, fmt.Errorf("list delivery pull requests: %w", err)
	}
	if err := iq.Select("i.repository_id, i.number, i.state, i.created_at").Order("i.repository_id, i.number").Find(&issues).Error; err != nil {
		return DeliveryResponse{}, fmt.Errorf("list delivery issues: %w", err)
	}
	if err := rq.Select("ar.id, ar.repository_id, ar.pr_number, ar.issue_numbers, ar.model, ar.harness, ar.billing_type, ar.input_tokens, ar.cached_input_tokens, ar.cache_write_tokens, ar.output_tokens, ar.reasoning_tokens, ar.started_at").Order("ar.started_at, ar.id").Find(&runs).Error; err != nil {
		return DeliveryResponse{}, fmt.Errorf("list delivery runs: %w", err)
	}
	eq := r.db.WithContext(ctx).Table("webhook_events we").Joins("JOIN repositories repo ON repo.id = we.repository_id").Where("repo.organization_id = ? AND we.received_at >= ? AND we.received_at < ?", org, *f.From, f.To.Add(24*time.Hour))
	if f.RepositoryID != "" {
		eq = eq.Where("we.repository_id = ?", f.RepositoryID)
	}
	if err := eq.Order("we.received_at ASC, we.delivery_id ASC").Find(&retained).Error; err != nil {
		return DeliveryResponse{}, fmt.Errorf("list delivery events: %w", err)
	}
	prices, err := r.loadPrices(ctx, org)
	if err != nil {
		return DeliveryResponse{}, err
	}
	out := DeliveryResponse{From: f.From.UTC().Format("2006-01-02"), To: f.To.UTC().Format("2006-01-02"), History: DeliveryHistory{Complete: false}}
	decoded := make([]event, 0, len(retained))
	diffs := map[string]int64{}
	for _, row := range retained {
		e := decode(row)
		decoded = append(decoded, e)
		if e.pr != nil && e.additions != nil && e.deletions != nil {
			diffs[string(row.RepositoryID)+":"+fmt.Sprint(*e.pr)] = *e.additions + *e.deletions
		}
	}
	out.SendBack = sendBack(decoded)
	out.FirstCI = firstCI(decoded)
	merged := map[string]bool{}
	closed := map[string]bool{}
	for _, p := range prs {
		k := p.RepositoryID + ":" + fmt.Sprint(p.Number)
		if p.MergedAt != nil {
			merged[k] = true
		}
		for _, n := range jsonNumbers(p.Closes) {
			if p.MergedAt != nil {
				closed[p.RepositoryID+":"+fmt.Sprint(n)] = true
			}
		}
	}
	for _, i := range issues {
		if i.State == "closed" {
			closed[i.RepositoryID+":"+fmt.Sprint(i.Number)] = true
		}
	}
	for _, run := range runs {
		c, ok := deliveryCost(run, prices)
		if !ok {
			out.History.Unavailable++
			continue
		}
		if run.PRNumber != nil && merged[run.RepositoryID+":"+fmt.Sprint(*run.PRNumber)] {
			out.MergedPRs.Eligible++
			out.MergedPRs.EquivalentCostUSD = add(out.MergedPRs.EquivalentCostUSD, c)
			if run.BillingType != "subscription" {
				out.MergedPRs.ActualCostUSD = add(out.MergedPRs.ActualCostUSD, c)
			}
			h := value(run.Harness)
			lines := int64(0)
			if run.PRNumber != nil {
				lines = diffs[run.RepositoryID+":"+fmt.Sprint(*run.PRNumber)]
			}
			observed := lines > 0
			addHarness(&out.Harness, h, c, run, lines, observed)
		}
		nums := jsonNumbers(run.IssueNumbers)
		for _, n := range nums {
			if closed[run.RepositoryID+":"+fmt.Sprint(n)] {
				out.ClosedIssues.Eligible++
				out.ClosedIssues.EquivalentCostUSD = add(out.ClosedIssues.EquivalentCostUSD, c)
				if run.BillingType != "subscription" {
					out.ClosedIssues.ActualCostUSD = add(out.ClosedIssues.ActualCostUSD, c)
				}
			}
		}
	}
	for _, p := range prs {
		if p.MergedAt != nil {
			out.MergedPRs.Count++
		}
	}
	for _, i := range issues {
		if i.State == "closed" {
			out.ClosedIssues.Count++
		}
	}
	finalizeCost(&out.MergedPRs)
	finalizeCost(&out.ClosedIssues)
	sort.Slice(out.Harness, func(i, j int) bool { return out.Harness[i].Harness < out.Harness[j].Harness })
	for i := range out.Harness {
		if out.Harness[i].ChangedLines > 0 && out.Harness[i].EquivalentCostUSD != nil {
			v := *out.Harness[i].EquivalentCostUSD / float64(out.Harness[i].ChangedLines)
			out.Harness[i].CostPerChangedLineUSD = &v
		}
	}
	return out, nil
}

func jsonNumbers(raw store.JSONB) []int64 { var n []int64; _ = json.Unmarshal(raw, &n); return n }
func deliveryCost(run deliveryRun, ps []priceRow) (float64, bool) {
	p, ok := priceAt(ps, run.Model, run.StartedAt)
	if !ok {
		return 0, false
	}
	return cost(AnalyticsRun{InputTokens: run.InputTokens, CachedInputTokens: run.CachedInputTokens, CacheWriteTokens: run.CacheWriteTokens, OutputTokens: run.OutputTokens, ReasoningTokens: run.ReasoningTokens}, p), true
}
func finalizeCost(m *DeliveryCostMetric) {
	if m.Eligible > 0 {
		v := *m.EquivalentCostUSD / float64(m.Eligible)
		m.CostPerItemUSD = &v
	}
}
func addHarness(ms *[]HarnessMetric, h string, c float64, r deliveryRun, lines int64, observed bool) {
	for i := range *ms {
		if (*ms)[i].Harness == h {
			x := &(*ms)[i]
			x.Runs++
			x.EquivalentCostUSD = add(x.EquivalentCostUSD, c)
			if observed {
				x.Eligible++
				x.ChangedLines += lines
				if lines > 0 {
					v := c / float64(lines)
					_ = v
				}
			} else {
				x.Unavailable++
			}
			return
		}
	}
	m := HarnessMetric{Harness: h, Runs: 1, EquivalentCostUSD: &c}
	if observed {
		m.Eligible = 1
		m.ChangedLines = lines
		if lines > 0 {
			v := c / float64(lines)
			m.CostPerChangedLineUSD = &v
		}
	} else {
		m.Unavailable = 1
	}
	*ms = append(*ms, m)
}
