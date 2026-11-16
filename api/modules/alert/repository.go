package alert

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Occurrence struct {
	ID, OrganizationID, AlertKey, Repository, Rule, Subject, Message string
	DetectedAt                                                       time.Time
}
type Delivery struct {
	ID, OccurrenceID, Channel, Error string
	Success                          bool
	DeliveredAt                      time.Time
}
type DeliveryResult struct {
	Success bool
}
type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Record(ctx context.Context, a Alert) (Occurrence, bool, error) {
	row := Occurrence{ID: uuid.NewString(), OrganizationID: a.OrganizationID, AlertKey: a.Key,
		Repository: a.Repository, Rule: a.Rule, Subject: a.Subject, Message: a.Message, DetectedAt: a.DetectedAt}
	query := `INSERT INTO alert_occurrences (id, organization_id, alert_key, repository, rule, subject, message, detected_at)
		SELECT ?, ?, ?, ?, ?, ?, ?, ? WHERE NOT EXISTS
		(SELECT 1 FROM alert_incident_states WHERE organization_id = ? AND alert_key = ? AND active)`
	result := r.db.WithContext(ctx).Exec(query, row.ID, row.OrganizationID, row.AlertKey, row.Repository,
		row.Rule, row.Subject, row.Message, row.DetectedAt, row.OrganizationID, row.AlertKey)
	if result.Error != nil {
		return Occurrence{}, false, fmt.Errorf("record alert occurrence: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		var persisted Occurrence
		if err := r.db.WithContext(ctx).Table("alert_occurrences").
			Where("organization_id = ? AND alert_key = ?", row.OrganizationID, row.AlertKey).Order("detected_at DESC").
			Take(&persisted).Error; err != nil {
			return Occurrence{}, false, fmt.Errorf("get recorded alert occurrence: %w", err)
		}
		return persisted, false, nil
	}
	if err := r.db.WithContext(ctx).Exec(`INSERT INTO alert_incident_states (organization_id, alert_key, active) VALUES (?, ?, TRUE)
		ON CONFLICT (organization_id, alert_key) DO UPDATE SET active = TRUE, updated_at = NOW()`, row.OrganizationID, row.AlertKey).Error; err != nil {
		return Occurrence{}, false, fmt.Errorf("activate alert incident: %w", err)
	}
	return row, true, nil
}

func (r *Repository) ReconcileStates(ctx context.Context, states []IncidentState, organizationID string) error {
	for _, state := range states {
		if state.Active {
			continue
		}
		if err := r.db.WithContext(ctx).Exec(`INSERT INTO alert_incident_states (organization_id, alert_key, active) VALUES (?, ?, ?)
			ON CONFLICT (organization_id, alert_key) DO UPDATE SET active = EXCLUDED.active, updated_at = NOW()`, organizationID, state.Key, state.Active).Error; err != nil {
			return fmt.Errorf("reconcile alert incident: %w", err)
		}
	}
	return nil
}

func (r *Repository) RecordDelivery(ctx context.Context, d Delivery) error {
	query := `INSERT INTO alert_deliveries (id, occurrence_id, channel, success, error, delivered_at)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (occurrence_id, channel) DO UPDATE SET
		success = EXCLUDED.success, error = EXCLUDED.error, delivered_at = EXCLUDED.delivered_at`
	result := r.db.WithContext(ctx).Exec(query, uuid.NewString(), d.OccurrenceID, d.Channel,
		d.Success, d.Error, d.DeliveredAt)
	if result.Error != nil {
		return fmt.Errorf("record alert delivery: %w", result.Error)
	}
	return nil
}

func (r *Repository) GetDelivery(ctx context.Context, occurrenceID, channel string) (DeliveryResult, bool, error) {
	var delivery DeliveryResult
	err := r.db.WithContext(ctx).Table("alert_deliveries").Select("success").
		Where("occurrence_id = ? AND channel = ?", occurrenceID, channel).Take(&delivery).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DeliveryResult{}, false, nil
	}
	if err != nil {
		return DeliveryResult{}, false, fmt.Errorf("get alert delivery: %w", err)
	}
	return delivery, true, nil
}

func (r *Repository) List(ctx context.Context, organizationID string, page, perPage int) ([]Occurrence, int64, error) {
	var total int64
	q := r.db.WithContext(ctx).Table("alert_occurrences").Where("organization_id = ?", organizationID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count alert history: %w", err)
	}
	var rows []Occurrence
	if err := q.Order("detected_at DESC, id DESC").Offset((page - 1) * perPage).Limit(perPage).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list alert history: %w", err)
	}
	return rows, total, nil
}
