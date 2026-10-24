package board

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) ListRepositories(ctx context.Context, page, perPage int) ([]RepositoryDTO, ListMeta, error) {
	q := r.db.WithContext(ctx).Model(&store.Repository{})
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, ListMeta{}, fmt.Errorf("count repositories: %w", err)
	}
	var rows []store.Repository
	if err := q.Order("full_name ASC").Offset((page - 1) * perPage).Limit(perPage).Find(&rows).Error; err != nil {
		return nil, ListMeta{}, fmt.Errorf("list repositories: %w", err)
	}
	items := make([]RepositoryDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, repositoryDTO(row))
	}
	return items, ListMeta{Page: page, PerPage: perPage, Total: total}, nil
}

func (r *Repository) RepositoryExists(ctx context.Context, id string) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&store.Repository{}).Where("id = ?", id).Count(&n).Error
	return n > 0, err
}

func (r *Repository) ListIssues(ctx context.Context, id string, page, perPage int) ([]IssueDTO, ListMeta, error) {
	q := r.db.WithContext(ctx).Model(&store.Issue{}).Where("repository_id = ?", id)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, ListMeta{}, fmt.Errorf("count issues: %w", err)
	}
	var rows []store.Issue
	if err := q.Order("number DESC").Offset((page - 1) * perPage).Limit(perPage).Find(&rows).Error; err != nil {
		return nil, ListMeta{}, fmt.Errorf("list issues: %w", err)
	}
	items := make([]IssueDTO, 0, len(rows))
	for _, row := range rows {
		labels := json.RawMessage(`[]`)
		if len(row.Labels) > 0 {
			if !json.Valid(row.Labels) {
				return nil, ListMeta{}, fmt.Errorf("decode issue labels: invalid JSON")
			}
			labels = append(json.RawMessage(nil), row.Labels...)
		}
		prs, err := r.listPRs(ctx, id, row.Number)
		if err != nil {
			return nil, ListMeta{}, err
		}
		items = append(items, IssueDTO{ID: string(row.ID), RepositoryID: string(row.RepositoryID), Number: row.Number, Title: row.Title, State: row.State, Status: row.Status, Labels: labels, Assignee: row.Assignee, PullRequests: prs})
	}
	return items, ListMeta{Page: page, PerPage: perPage, Total: total}, nil
}

func (r *Repository) listPRs(ctx context.Context, id string, issue int64) ([]PullRequestDTO, error) {
	var rows []store.PullRequest
	if err := r.db.WithContext(ctx).Where("repository_id = ? AND closes @> ?::jsonb", id, fmt.Sprintf("[%d]", issue)).Order("number DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list pull requests: %w", err)
	}
	items := make([]PullRequestDTO, 0, len(rows))
	for _, row := range rows {
		var runs []store.PipelineRun
		if err := r.db.WithContext(ctx).Where("repository_id = ? AND sha = ?", id, row.HeadSHA).Order("started_at DESC NULLS LAST").Find(&runs).Error; err != nil {
			return nil, fmt.Errorf("list pipeline runs: %w", err)
		}
		out := make([]PipelineRunDTO, 0, len(runs))
		for _, run := range runs {
			out = append(out, PipelineRunDTO{ExternalID: run.ExternalID, Status: run.Status, Conclusion: run.Conclusion, SHA: run.SHA})
		}
		items = append(items, PullRequestDTO{Number: row.Number, Title: row.Title, State: row.State, HeadSHA: row.HeadSHA, Branch: row.Branch, PipelineRuns: out})
	}
	return items, nil
}
