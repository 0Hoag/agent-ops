package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PullRequestRepository struct{ db *gorm.DB }

func (repository *PullRequestRepository) Upsert(ctx context.Context, item *PullRequest) error {
	err := repository.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "repository_id"}, {Name: "number"}}, DoUpdates: clause.Assignments(map[string]any{
		"title": gorm.Expr("EXCLUDED.title"), "state": gorm.Expr("EXCLUDED.state"), "head_sha": gorm.Expr("EXCLUDED.head_sha"), "branch": gorm.Expr("EXCLUDED.branch"), "closes": gorm.Expr("EXCLUDED.closes"), "merged_at": gorm.Expr("EXCLUDED.merged_at"), "updated_at": gorm.Expr("NOW()"),
	})}).Create(item).Error
	if err != nil {
		return fmt.Errorf("upsert pull request %d: %w", item.Number, err)
	}
	return nil
}

func (repository *PullRequestRepository) Get(ctx context.Context, id UUID) (*PullRequest, error) {
	var item PullRequest
	if err := repository.db.WithContext(ctx).Where("id = ?", id).First(&item).Error; err != nil {
		return nil, fmt.Errorf("get pull request %s: %w", id, err)
	}
	return &item, nil
}

func (repository *PullRequestRepository) GetByNumber(ctx context.Context, repositoryID UUID, number int64) (*PullRequest, error) {
	var item PullRequest
	if err := repository.db.WithContext(ctx).Where("repository_id = ? AND number = ?", repositoryID, number).First(&item).Error; err != nil {
		return nil, fmt.Errorf("get pull request %d: %w", number, err)
	}
	return &item, nil
}

func (repository *PullRequestRepository) ListByClosedIssue(ctx context.Context, repositoryID UUID, issueNumber int64) ([]PullRequest, error) {
	var items []PullRequest
	query := "repository_id = ? AND closes @> ?::jsonb"
	if err := repository.db.WithContext(ctx).Where(query, repositoryID, fmt.Sprintf("[%d]", issueNumber)).Order("number DESC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list pull requests closing issue %d: %w", issueNumber, err)
	}
	return items, nil
}

func (repository *PullRequestRepository) List(ctx context.Context, repositoryID UUID, page, perPage int) ([]PullRequest, Page, error) {
	page, perPage = pageArgs(page, perPage)
	query := repository.db.WithContext(ctx).Model(&PullRequest{}).Where("repository_id = ?", repositoryID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, Page{}, fmt.Errorf("count pull requests: %w", err)
	}
	var items []PullRequest
	if err := query.Order("number DESC").Offset((page - 1) * perPage).Limit(perPage).Find(&items).Error; err != nil {
		return nil, Page{}, fmt.Errorf("list pull requests: %w", err)
	}
	return items, Page{Page: page, PerPage: perPage, Total: total}, nil
}
