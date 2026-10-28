package agentrun

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"gorm.io/gorm"
)

var ErrInvalidPrice = errors.New("invalid price version")

type PriceVersion struct {
	ID             store.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	OrganizationID store.UUID `json:"organization_id" gorm:"type:uuid;not null"`
	Model          string     `json:"model" gorm:"not null"`
	EffectiveDate  time.Time  `json:"effective_date" gorm:"type:date;not null"`
	Price
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (PriceVersion) TableName() string { return "agent_run_price_versions" }
func validatePrice(p *PriceVersion) error {
	if strings.TrimSpace(p.Model) == "" || p.EffectiveDate.IsZero() || p.EffectiveDate.UTC().Format("15:04:05") != "00:00:00" {
		return ErrInvalidPrice
	}
	if p.InputPerMillion < 0 || p.CachedInputPerMillion < 0 || p.CacheWritePerMillion < 0 || p.OutputPerMillion < 0 || p.ReasoningPerMillion < 0 {
		return ErrInvalidPrice
	}
	return nil
}
func (r *Repository) ListPrices(ctx context.Context, org string) ([]PriceVersion, error) {
	var out []PriceVersion
	err := r.db.WithContext(ctx).Where("organization_id = ?", org).Order("model, effective_date DESC").Find(&out).Error
	return out, err
}
func (r *Repository) CreatePrice(ctx context.Context, p *PriceVersion) error {
	if err := validatePrice(p); err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(p).Error; err != nil {
		return fmt.Errorf("create price version: %w", err)
	}
	return nil
}
func (r *Repository) UpdatePrice(ctx context.Context, org, id string, p *PriceVersion) error {
	if err := validatePrice(p); err != nil {
		return err
	}
	result := r.db.WithContext(ctx).Model(&PriceVersion{}).Where("id = ? AND organization_id = ?", id, org).Updates(map[string]any{"model": p.Model, "effective_date": p.EffectiveDate, "input_per_million": p.InputPerMillion, "cached_input_per_million": p.CachedInputPerMillion, "cache_write_per_million": p.CacheWritePerMillion, "output_per_million": p.OutputPerMillion, "reasoning_per_million": p.ReasoningPerMillion, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
func (r *Repository) priceAt(ctx context.Context, org, model string, day time.Time) (*Price, error) {
	var p PriceVersion
	err := r.db.WithContext(ctx).Where("organization_id = ? AND model = ? AND effective_date <= ?", org, model, day.UTC().Format("2006-01-02")).Order("effective_date DESC").First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p.Price, nil
}
