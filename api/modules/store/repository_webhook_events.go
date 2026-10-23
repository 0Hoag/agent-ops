package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WebhookEventRepository struct{ db *gorm.DB }

// CreateIfNew is idempotent for a delivery within repository/provider scope.
func (repository *WebhookEventRepository) CreateIfNew(ctx context.Context, item *WebhookEvent) (bool, error) {
	result := repository.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(item)
	if result.Error != nil {
		return false, fmt.Errorf("create webhook event %s: %w", item.DeliveryID, result.Error)
	}
	return result.RowsAffected == 1, nil
}

func (repository *WebhookEventRepository) GetByDelivery(ctx context.Context, repositoryID UUID, provider, deliveryID string) (*WebhookEvent, error) {
	var item WebhookEvent
	if err := repository.db.WithContext(ctx).Where("repository_id = ? AND provider = ? AND delivery_id = ?", repositoryID, provider, deliveryID).First(&item).Error; err != nil {
		return nil, fmt.Errorf("get webhook event %s: %w", deliveryID, err)
	}
	return &item, nil
}

func (repository *WebhookEventRepository) List(ctx context.Context, repositoryID UUID, page, perPage int) ([]WebhookEvent, Page, error) {
	page, perPage = pageArgs(page, perPage)
	query := repository.db.WithContext(ctx).Model(&WebhookEvent{}).Where("repository_id = ?", repositoryID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, Page{}, fmt.Errorf("count webhook events: %w", err)
	}
	var items []WebhookEvent
	if err := query.Order("received_at DESC").Offset((page - 1) * perPage).Limit(perPage).Find(&items).Error; err != nil {
		return nil, Page{}, fmt.Errorf("list webhook events: %w", err)
	}
	return items, Page{Page: page, PerPage: perPage, Total: total}, nil
}
