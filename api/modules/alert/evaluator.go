package alert

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Evaluator struct {
	thresholds Thresholds
	mu         sync.Mutex
	active     map[string]bool
}

func NewEvaluator(thresholds Thresholds) *Evaluator {
	return &Evaluator{thresholds: thresholds, active: make(map[string]bool)}
}

func (e *Evaluator) Evaluate(ctx context.Context, now time.Time, observation Observation) []Alert {
	if err := ctx.Err(); err != nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	alerts := make([]Alert, 0)
	add := func(rule, subject, message string, qualifies bool) {
		key := fmt.Sprintf("%s|%s|%s|%s", observation.OrganizationID, observation.Repository, rule, subject)
		if !qualifies {
			delete(e.active, key)
			return
		}
		if e.active[key] {
			return
		}
		e.active[key] = true
		alerts = append(alerts, Alert{Key: key, OrganizationID: observation.OrganizationID, Repository: observation.Repository, Rule: rule, Subject: subject, DetectedAt: now, Message: message})
	}
	for _, run := range observation.Runs {
		denominator := run.InputTokens + run.CachedInputTokens
		hit := float64(run.CachedInputTokens) / float64(denominator)
		add(RuleLowCache, run.Worker, "cache hit below threshold", denominator > 0 && hit < e.thresholds.MinCacheHit)
		last := run.LastActivityAt
		if last.IsZero() {
			last = run.StartedAt
		}
		add(RuleSilent, run.Worker, "active worker has no recent activity", run.Active && !last.IsZero() && now.Sub(last) > e.thresholds.SilentAfter)
	}
	for _, pipeline := range observation.Pipelines {
		add(RuleRedCI, pipeline.ID, "completed main-branch CI failed", pipeline.Branch == "main" && pipeline.Status == "completed" && pipeline.Conclusion != "success" && pipeline.Conclusion != "skipped")
	}
	for _, pr := range observation.PullRequests {
		add(RuleStalePR, fmt.Sprint(pr.Number), "open pull request is stale", pr.State == "open" && !pr.Merged && !pr.UpdatedAt.IsZero() && now.Sub(pr.UpdatedAt) > e.thresholds.PRStaleAfter)
	}
	return alerts
}
