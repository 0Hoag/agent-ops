package work

import (
	"context"
	"fmt"

	"github.com/0Hoag/agent-ops/api/modules/agentrun"
	"gorm.io/gorm"
)

type Repository struct {
	db     *gorm.DB
	prices *agentrun.Repository
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db, prices: agentrun.NewRepository(db)}
}

func (r *Repository) repoOK(ctx context.Context, id string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Table("repositories").Where("id = ? AND organization_id = ?", id, org(ctx)).Count(&n).Error
	return n == 1, err
}
func org(ctx context.Context) string { v, _ := ctx.Value(organizationKey).(string); return v }
func (r *Repository) query(ctx context.Context, id, kind, state, q string) *gorm.DB {
	base := `SELECT x.kind, x.number, x.title, '' AS url, '' AS author, x.state, x.ci_status, x.created_at, x.updated_at, COALESCE(a.runs_count,0) runs_count, a.cost_usd, COALESCE(a.tokens,0) tokens, GREATEST(x.updated_at, COALESCE(a.last_activity,x.updated_at)) last_activity FROM (SELECT 'pr' kind, p.number,p.title,p.state,COALESCE((SELECT conclusion FROM pipeline_runs c WHERE c.repository_id=p.repository_id AND c.pr_number=p.number ORDER BY c.started_at DESC NULLS LAST LIMIT 1),'') ci_status,p.created_at,p.updated_at,p.repository_id FROM pull_requests p WHERE p.repository_id=? UNION ALL SELECT 'issue',i.number,i.title,i.state,'',i.created_at,i.updated_at,i.repository_id FROM issues i WHERE i.repository_id=?) x LEFT JOIN (SELECT repository_id, CASE WHEN pr_number IS NULL THEN 'issue' ELSE 'pr' END kind, COALESCE(pr_number,0) number, COUNT(*) runs_count, SUM(input_tokens+cached_input_tokens+cache_write_tokens+output_tokens+reasoning_tokens) tokens, MAX(ended_at) last_activity FROM agent_runs GROUP BY repository_id, CASE WHEN pr_number IS NULL THEN 'issue' ELSE 'pr' END, COALESCE(pr_number,0)) a ON a.repository_id=x.repository_id AND a.kind=x.kind AND a.number=x.number WHERE EXISTS (SELECT 1 FROM repositories WHERE id=x.repository_id AND organization_id=?)`
	args := []any{id, id, org(ctx)}
	if kind != "" {
		base += " AND x.kind=?"
		args = append(args, kind)
	}
	if state != "" {
		base += " AND x.state=?"
		args = append(args, state)
	}
	if q != "" {
		base += " AND x.title ILIKE ?"
		args = append(args, "%"+q+"%")
	}
	base += " ORDER BY last_activity DESC, x.kind, x.number DESC"
	return r.db.WithContext(ctx).Raw(base, args...)
}
func (r *Repository) List(ctx context.Context, id, state, q string, page, per int) ([]Item, int64, error) {
	ok, e := r.repoOK(ctx, id)
	if e != nil {
		return nil, 0, e
	}
	if !ok {
		return nil, 0, gorm.ErrRecordNotFound
	}
	var total int64
	count := r.db.WithContext(ctx).Table("(SELECT 1 FROM pull_requests WHERE repository_id=? UNION ALL SELECT 1 FROM issues WHERE repository_id=?) x", id, id)
	if state != "" {
		count = count.Where("1=1")
	}
	if e = count.Count(&total).Error; e != nil {
		return nil, 0, e
	}
	var out []Item
	if e = r.query(ctx, id, "", state, q).Offset((page - 1) * per).Limit(per).Scan(&out).Error; e != nil {
		return nil, 0, e
	}
	return out, total, nil
}
func (r *Repository) Detail(ctx context.Context, id, kind string, n int64) (*Detail, error) {
	ok, e := r.repoOK(ctx, id)
	if e != nil || !ok {
		return nil, gorm.ErrRecordNotFound
	}
	var items []Item
	if e = r.query(ctx, id, kind, "", "").Where("x.number = ?", n).Scan(&items).Error; e != nil {
		return nil, e
	}
	if len(items) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var runs []Run
	where := "repository_id=? AND "
	args := []any{id}
	if kind == "pr" {
		where += "pr_number=?"
	} else {
		where += "pr_number IS NULL AND issue_numbers @> ?::jsonb"
		args = append(args, fmt.Sprintf("[%d]", n))
	}
	if kind == "pr" {
		args = append(args, n)
	}
	rows := []agentrun.AgentRun{}
	if e = r.db.WithContext(ctx).Where(where, args...).Order("started_at DESC").Find(&rows).Error; e != nil {
		return nil, e
	}
	for _, v := range rows {
		t := v.InputTokens + v.CachedInputTokens + v.CacheWriteTokens + v.OutputTokens + v.ReasoningTokens
		runs = append(runs, Run{ID: string(v.ID), Model: v.Model, StartedAt: v.StartedAt, EndedAt: v.EndedAt, CostUSD: v.CostUSD, Tokens: t, CacheHit: v.CachedInputTokens > 0})
	}
	return &Detail{Item: items[0], Runs: runs}, nil
}
