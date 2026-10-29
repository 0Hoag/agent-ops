package budget

import (
	"context"
	"github.com/0Hoag/agent-ops/api/modules/store"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) List(ctx context.Context, org string) ([]Budget, error) {
	var v []Budget
	e := r.db.WithContext(ctx).Where("organization_id = ?", org).Order("starts_at DESC").Find(&v).Error
	return v, e
}
func (r *Repository) Get(ctx context.Context, org, id string) (*Budget, error) {
	var v Budget
	e := r.db.WithContext(ctx).Where("organization_id = ? AND id = ?", org, id).First(&v).Error
	return &v, e
}
func (r *Repository) Create(ctx context.Context, v *Budget) error {
	return r.db.WithContext(ctx).Create(v).Error
}
func (r *Repository) Update(ctx context.Context, org, id string, v *Budget) error {
	q := r.db.WithContext(ctx).Model(&Budget{}).Where("organization_id = ? AND id = ?", org, id).Updates(map[string]any{"repository_id": v.RepositoryID, "wave": v.Wave, "token_limit": v.TokenLimit, "usd_limit": v.USDLimit, "starts_at": v.StartsAt, "ends_at": v.EndsAt, "updated_at": gorm.Expr("NOW()")})
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
func (r *Repository) Delete(ctx context.Context, org, id string) error {
	q := r.db.WithContext(ctx).Where("organization_id = ? AND id = ?", org, id).Delete(&Budget{})
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
func (r *Repository) RepositoryBelongs(ctx context.Context, org, id string) (bool, error) {
	var n int64
	e := r.db.WithContext(ctx).Table("repositories").Where("id = ? AND organization_id = ?", store.UUID(id), org).Count(&n).Error
	return n == 1, e
}
