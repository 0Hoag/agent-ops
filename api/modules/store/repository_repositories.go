package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
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

// UpsertConfigured preserves tenant-owned settings and only creates a new row
// when the configuration supplies an explicit, valid organization UUID.
func (repository *RepositoryRepository) UpsertConfigured(ctx context.Context, item *Repository) error {
	_, err := repository.GetByName(ctx, item.Provider, item.FullName)
	if err == nil {
		return nil
	}
	if !isRecordNotFound(err) {
		return err
	}
	if _, parseErr := uuid.Parse(item.OrganizationID); parseErr != nil {
		return fmt.Errorf("configured repository %s/%s requires a valid organization_id for a new row", item.Provider, item.FullName)
	}
	return repository.Create(ctx, item)
}

func isRecordNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

func (repository *RepositoryRepository) Get(ctx context.Context, id UUID) (*Repository, error) {
	var item Repository
	if err := repository.db.WithContext(ctx).Where("id = ?", id).First(&item).Error; err != nil {
		return nil, fmt.Errorf("get repository %s: %w", id, err)
	}
	return &item, nil
}

func (repository *RepositoryRepository) GetByName(ctx context.Context, provider, fullName string) (*Repository, error) {
	var item Repository
	if err := repository.db.WithContext(ctx).Where("provider = ? AND full_name = ?", provider, fullName).First(&item).Error; err != nil {
		return nil, fmt.Errorf("get repository %s/%s: %w", provider, fullName, err)
	}
	return &item, nil
}

func (repository *RepositoryRepository) GetByOrganization(ctx context.Context, organizationID, id UUID) (*Repository, error) {
	var item Repository
	if err := repository.db.WithContext(ctx).Where("id = ? AND organization_id = ?", id, organizationID).First(&item).Error; err != nil {
		return nil, fmt.Errorf("get organization repository: %w", err)
	}
	return &item, nil
}
func (repository *RepositoryRepository) ListByOrganization(ctx context.Context, organizationID string) ([]Repository, error) {
	var items []Repository
	if err := repository.db.WithContext(ctx).Where("organization_id = ?", organizationID).Order("full_name ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list organization repositories: %w", err)
	}
	return items, nil
}
func (repository *RepositoryRepository) DeleteByOrganization(ctx context.Context, organizationID string, id UUID) error {
	if result := repository.db.WithContext(ctx).Where("id = ? AND organization_id = ?", id, organizationID).Delete(&Repository{}); result.Error != nil {
		return fmt.Errorf("delete repository: %w", result.Error)
	} else if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
func (repository *RepositoryRepository) Update(ctx context.Context, item *Repository) error {
	if err := repository.db.WithContext(ctx).Save(item).Error; err != nil {
		return fmt.Errorf("update repository: %w", err)
	}
	return nil
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
