package work

import (
	"context"
	"fmt"
	"strconv"

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
func (r *Repository) query(ctx context.Context, id, kind, state, q string, number *int64) *gorm.DB {
	base := `WITH work AS (
SELECT 'pr' kind, p.number, p.title, p.state,
 COALESCE((SELECT conclusion FROM pipeline_runs c WHERE c.repository_id=p.repository_id AND c.pr_number=p.number ORDER BY c.started_at DESC NULLS LAST LIMIT 1),'') ci_status,
 p.created_at, p.updated_at, p.repository_id FROM pull_requests p WHERE p.repository_id=?
UNION ALL SELECT 'issue', i.number, i.title, i.state, '', i.created_at, i.updated_at, i.repository_id
 FROM issues i WHERE i.repository_id=?
), run_costs AS (
	 SELECT ar.repository_id, ar.kind, ar.number, COUNT(*) runs_count,
  SUM(ar.input_tokens+ar.cached_input_tokens+ar.cache_write_tokens+ar.output_tokens+ar.reasoning_tokens) tokens,
  MAX(ar.ended_at) last_activity,
	 CASE WHEN COUNT(ar.cost_usd) = COUNT(*) THEN SUM(ar.cost_usd) END cost_usd
	 FROM (SELECT a.*, 'pr' kind, a.pr_number number FROM agent_runs a WHERE a.pr_number IS NOT NULL
	       UNION ALL SELECT DISTINCT a.*, 'issue' kind, issue_number::bigint number
	       FROM agent_runs a CROSS JOIN LATERAL jsonb_array_elements_text(a.issue_numbers) issue_number
	       WHERE jsonb_array_length(a.issue_numbers)>0) ar
	 LEFT JOIN repositories repo ON repo.id=ar.repository_id
 WHERE ar.repository_id=? AND repo.organization_id=?
	 GROUP BY ar.repository_id, ar.kind, ar.number
)
SELECT x.kind, x.number, x.title,
 CASE repo.provider
  WHEN 'github' THEN 'https://github.com/' || repo.full_name || CASE WHEN x.kind='pr' THEN '/pull/' ELSE '/issues/' END || x.number::text
  WHEN 'gitlab' THEN regexp_replace(COALESCE(NULLIF(repo.gitlab_base_url,''),'https://gitlab.com'), '/+$', '') || '/' || repo.full_name || '/-/' || CASE WHEN x.kind='pr' THEN 'merge_requests/' ELSE 'issues/' END || x.number::text
  ELSE '' END AS url,
 '' AS author, x.state, x.ci_status, x.created_at, x.updated_at,
 COALESCE(a.runs_count,0) runs_count, a.cost_usd, COALESCE(a.tokens,0) tokens,
 GREATEST(x.updated_at, COALESCE(a.last_activity,x.updated_at)) last_activity
FROM work x JOIN repositories repo ON repo.id=x.repository_id LEFT JOIN run_costs a ON a.repository_id=x.repository_id AND a.kind=x.kind AND a.number=x.number
WHERE EXISTS (SELECT 1 FROM repositories WHERE id=x.repository_id AND organization_id=?)`
	args := []any{id, id, id, org(ctx), org(ctx)}
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
	if number != nil {
		base += " AND x.number = ?"
		args = append(args, *number)
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
	countQuery := r.query(ctx, id, "", state, q, nil)
	if e = r.db.WithContext(ctx).Raw(
		"SELECT COUNT(*) FROM ("+countQuery.Statement.SQL.String()+") AS work_count",
		countQuery.Statement.Vars...,
	).Scan(&total).Error; e != nil {
		return nil, 0, e
	}
	var out []Item
	listQuery := r.query(ctx, id, "", state, q, nil)
	if e = r.db.WithContext(ctx).Raw(
		listQuery.Statement.SQL.String()+" LIMIT "+strconv.Itoa(per)+" OFFSET "+strconv.Itoa((page-1)*per),
		listQuery.Statement.Vars...,
	).Scan(&out).Error; e != nil {
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
	if e = r.query(ctx, id, kind, "", "", &n).Scan(&items).Error; e != nil {
		return nil, e
	}
	if len(items) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var runs []Run
	where, args := runAttribution(kind, id, n)
	type runRow struct {
		agentrun.AgentRun
	}
	rows := []runRow{}
	selectRuns := `agent_runs.*`
	runQuery := r.db.WithContext(ctx).Table("agent_runs").Select(selectRuns).
		Joins("JOIN repositories ON repositories.id=agent_runs.repository_id").
		Where("agent_runs."+where, args...).Order("agent_runs.started_at DESC")
	if e = runQuery.Find(&rows).Error; e != nil {
		return nil, e
	}
	for _, v := range rows {
		t := v.InputTokens + v.CachedInputTokens + v.CacheWriteTokens + v.OutputTokens + v.ReasoningTokens
		runs = append(runs, Run{ID: string(v.ID), Model: v.Model, StartedAt: v.StartedAt, EndedAt: v.EndedAt, CostUSD: v.CostUSD, Tokens: t, CacheHit: v.CachedInputTokens > 0})
	}
	return &Detail{Item: items[0], Runs: runs}, nil
}

func runAttribution(kind, id string, n int64) (string, []any) {
	if kind == "pr" {
		return "repository_id=? AND pr_number=?", []any{id, n}
	}
	return "repository_id=? AND issue_numbers @> ?::jsonb", []any{id, fmt.Sprintf("[%d]", n)}
}
