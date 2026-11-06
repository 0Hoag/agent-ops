package alert

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrInvalidThresholds = errors.New("invalid alert thresholds")

// GetThresholds returns nil when the organization has no saved configuration.
func (r *Repository) GetThresholds(ctx context.Context, organizationID string) (*Thresholds, error) {
	if err := validateOrganizationID(organizationID); err != nil {
		return nil, err
	}
	var row struct {
		OrganizationID string
		MinCacheHit    float64
		SilentSeconds  int64
		StaleSeconds   int64
	}
	err := r.db.WithContext(ctx).Table("alert_thresholds").Where("organization_id = ?", organizationID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get alert thresholds: %w", err)
	}
	return &Thresholds{MinCacheHit: row.MinCacheHit, SilentAfter: time.Duration(row.SilentSeconds) * time.Second, PRStaleAfter: time.Duration(row.StaleSeconds) * time.Second}, nil
}

func (r *Repository) UpsertThresholds(ctx context.Context, organizationID string, thresholds Thresholds) error {
	if err := validateOrganizationID(organizationID); err != nil {
		return err
	}
	if thresholds.MinCacheHit < 0 || thresholds.MinCacheHit > 1 || thresholds.SilentAfter <= 0 || thresholds.PRStaleAfter <= 0 || thresholds.SilentAfter%time.Second != 0 || thresholds.PRStaleAfter%time.Second != 0 {
		return ErrInvalidThresholds
	}
	const query = `INSERT INTO alert_thresholds (organization_id, min_cache_hit, silent_seconds, stale_seconds)
		VALUES (?, ?, ?, ?) ON CONFLICT (organization_id) DO UPDATE SET
		min_cache_hit = EXCLUDED.min_cache_hit, silent_seconds = EXCLUDED.silent_seconds, stale_seconds = EXCLUDED.stale_seconds, updated_at = NOW()`
	if err := r.db.WithContext(ctx).Exec(query, organizationID, thresholds.MinCacheHit, int64(thresholds.SilentAfter/time.Second), int64(thresholds.PRStaleAfter/time.Second)).Error; err != nil {
		return fmt.Errorf("upsert alert thresholds: %w", err)
	}
	return nil
}

func validateOrganizationID(id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("%w: organization id: %v", ErrInvalidThresholds, err)
	}
	return nil
}
