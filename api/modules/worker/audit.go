package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"gorm.io/gorm"
)

type ActionAudit struct {
	ID             store.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID string     `gorm:"type:uuid;not null;index"`
	ActorID        string     `gorm:"not null"`
	OccurredAt     time.Time  `gorm:"not null"`
	Action         string     `gorm:"not null"`
	Wave           string     `gorm:"not null"`
	JobID          string     `gorm:"not null"`
	Outcome        string     `gorm:"not null"`
	CreatedAt      time.Time  `gorm:"not null"`
}
type AuditRepository interface {
	Create(context.Context, *ActionAudit) error
}
type AuditReader interface {
	List(context.Context, string, int, int) ([]ActionAudit, int64, error)
}
type auditRepository struct{ db *gorm.DB }

func NewAuditRepository(db *gorm.DB) AuditRepository { return &auditRepository{db: db} }
func (r *auditRepository) Create(ctx context.Context, audit *ActionAudit) error {
	if err := r.db.WithContext(ctx).Create(audit).Error; err != nil {
		return fmt.Errorf("create worker action audit: %w", err)
	}
	return nil
}

func (r *auditRepository) List(ctx context.Context, organizationID string, page, perPage int) ([]ActionAudit, int64, error) {
	query := r.db.WithContext(ctx).Where("organization_id = ?", organizationID)
	var total int64
	if err := query.Model(&ActionAudit{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count worker action audits: %w", err)
	}
	var rows []ActionAudit
	if err := query.Order("occurred_at DESC, id DESC").Offset((page - 1) * perPage).Limit(perPage).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list worker action audits: %w", err)
	}
	return rows, total, nil
}
