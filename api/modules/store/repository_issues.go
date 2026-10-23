package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type IssueRepository struct{ db *gorm.DB }

func (repository *IssueRepository) Upsert(ctx context.Context, item *Issue) error {
	err := repository.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "repository_id"}, {Name: "number"}},
		DoUpdates: clause.Assignments(map[string]any{
			"title": gorm.Expr("EXCLUDED.title"), "state": gorm.Expr("EXCLUDED.state"), "status": gorm.Expr("EXCLUDED.status"), "labels": gorm.Expr("EXCLUDED.labels"), "assignee": gorm.Expr("EXCLUDED.assignee"), "updated_at": gorm.Expr("NOW()"),
		}),
	}).Create(item).Error
	if err != nil {
		return fmt.Errorf("upsert issue %d: %w", item.Number, err)
	}
	return nil
}

func (repository *IssueRepository) Get(ctx context.Context, id UUID) (*Issue, error) {
	var item Issue
	if err := repository.db.WithContext(ctx).Where("id = ?", id).First(&item).Error; err != nil {
		return nil, fmt.Errorf("get issue %s: %w", id, err)
	}
	return &item, nil
}

func (repository *IssueRepository) GetByNumber(ctx context.Context, repositoryID UUID, number int64) (*Issue, error) {
	var item Issue
	if err := repository.db.WithContext(ctx).Where("repository_id = ? AND number = ?", repositoryID, number).First(&item).Error; err != nil {
		return nil, fmt.Errorf("get issue %d: %w", number, err)
	}
	return &item, nil
}

func (repository *IssueRepository) List(ctx context.Context, repositoryID UUID, page, perPage int) ([]Issue, Page, error) {
	page, perPage = pageArgs(page, perPage)
	query := repository.db.WithContext(ctx).Model(&Issue{}).Where("repository_id = ?", repositoryID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, Page{}, fmt.Errorf("count issues: %w", err)
	}
	var items []Issue
	if err := query.Order("number DESC").Offset((page - 1) * perPage).Limit(perPage).Find(&items).Error; err != nil {
		return nil, Page{}, fmt.Errorf("list issues: %w", err)
	}
	return items, Page{Page: page, PerPage: perPage, Total: total}, nil
}
