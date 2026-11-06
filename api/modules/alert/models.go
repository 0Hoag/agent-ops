package alert

import "time"

// ThresholdsModel is the persisted organization-scoped alert configuration.
// Durations are stored as integer seconds by the repository.
type ThresholdsModel struct {
	OrganizationID string        `json:"organization_id" gorm:"type:uuid;primaryKey"`
	MinCacheHit    float64       `json:"min_cache_hit"`
	SilentAfter    time.Duration `json:"silent_after" gorm:"-"`
	PRStaleAfter   time.Duration `json:"pr_stale_after" gorm:"-"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

func (ThresholdsModel) TableName() string { return "alert_thresholds" }
