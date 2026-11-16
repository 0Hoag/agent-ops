package alert

import (
	"context"
	"fmt"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/budget"
)

func (r *Runtime) budgetObservations(ctx context.Context, repo struct{ ID, OrganizationID, FullName string }, now time.Time) ([]BudgetObservation, error) {
	limits, err := budget.NewRepository(r.db).List(ctx, repo.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("list alert budgets: %w", err)
	}
	out := make([]BudgetObservation, 0, len(limits))
	for _, limit := range limits {
		if now.Before(limit.StartsAt) || !now.Before(limit.EndsAt) || (limit.RepositoryID != nil && string(*limit.RepositoryID) != repo.ID) {
			continue
		}
		query := r.db.WithContext(ctx).Table("agent_runs ar").Joins("JOIN repositories repo ON repo.id = ar.repository_id").Where("repo.organization_id = ? AND ar.started_at >= ? AND ar.started_at < ?", repo.OrganizationID, limit.StartsAt, limit.EndsAt)
		if limit.RepositoryID != nil {
			query = query.Where("ar.repository_id = ?", repo.ID)
		}
		if limit.Wave != "" {
			query = query.Where("ar.wave = ?", limit.Wave)
		}
		var usage struct {
			Tokens int64
			USD    *float64
		}
		if err := query.Select("COALESCE(SUM(ar.input_tokens + ar.cached_input_tokens + ar.cache_write_tokens + ar.output_tokens + ar.reasoning_tokens), 0) AS tokens, SUM(ar.cost_usd) AS usd").Scan(&usage).Error; err != nil {
			return nil, fmt.Errorf("sum alert budget usage: %w", err)
		}
		out = append(out, BudgetObservation{ID: string(limit.ID), Tokens: usage.Tokens, USD: usage.USD, Limit: limit})
	}
	return out, nil
}
