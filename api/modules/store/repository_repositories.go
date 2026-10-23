package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RepositoryRepository struct{ db *gorm.DB }

func (repository *RepositoryRepository) Create(ctx context.Context, item *Repository) error {
	if err := repository.db.WithContext(ctx).Create(item).Error; err != nil {
		return fmt.Errorf("create repository: %w", err)
	}
	return nil
}

func (repository *RepositoryRepository) Upsert(ctx context.Context, item *Repository) error {
	err := repository.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "provider"}, {Name: "full_name"}},
		DoUpdates: clause.Assignments(map[string]any{
			"webhook_secret_env_ref": gorm.Expr("COALESCE(NULLIF(EXCLUDED.webhook_secret_env_ref, ''), repositories.webhook_secret_env_ref)"),
			"updated_at":             gorm.Expr("NOW()"),
		}),
	}).Create(item).Error
	if err != nil {
		return fmt.Errorf("upsert repository: %w", err)
	}
	return nil
}

func (repository *RepositoryRepository) Get(ctx context.Context, id UUID) (*Repository, error) {
	var item Repository
	if err := repository.db.WithContext(ctx).Where("id = ?", id).First(&item).Error; err != nil {
		return nil, fmt.Errorf("get repository %s: %w", id, err)
	}
	return &item, nil
}

func (repository *RepositoryRepository) List(ctx context.Context, page, perPage int) ([]Repository, Page, error) {
	page, perPage = pageArgs(page, perPage)
	query := repository.db.WithContext(ctx).Model(&Repository{})
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, Page{}, fmt.Errorf("count repositories: %w", err)
	}
	var items []Repository
	if err := query.Order("full_name ASC").Offset((page - 1) * perPage).Limit(perPage).Find(&items).Error; err != nil {
		return nil, Page{}, fmt.Errorf("list repositories: %w", err)
	}
	return items, Page{Page: page, PerPage: perPage, Total: total}, nil
}
