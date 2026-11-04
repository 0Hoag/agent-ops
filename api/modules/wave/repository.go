package wave

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, org string, page, perPage int) ([]Wave, int64, error) {
	q := r.db.WithContext(ctx).Where("organization_id = ?", org)
	var total int64
	if err := q.Model(&Wave{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count waves: %w", err)
	}
	var rows []Wave
	if err := q.Order("created_at DESC").Offset((page - 1) * perPage).Limit(perPage).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list waves: %w", err)
	}
	return rows, total, nil
}
func (r *Repository) Get(ctx context.Context, org, id string) (*Wave, error) {
	var row Wave
	err := r.db.WithContext(ctx).Where("organization_id = ? AND id = ?", org, id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get wave: %w", err)
	}
	return &row, nil
}
func (r *Repository) Create(ctx context.Context, org, name, brief string) (*Wave, error) {
	name, brief = strings.TrimSpace(name), strings.TrimSpace(brief)
	if name == "" || brief == "" {
		return nil, errors.New("name and brief are required")
	}
	row := &Wave{ID: uuid.NewString(), OrganizationID: org, Name: name, Brief: brief, State: Todo}
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, fmt.Errorf("create wave: %w", err)
	}
	return row, nil
}
func (r *Repository) Update(ctx context.Context, org, id, name, brief, state string) (*Wave, error) {
	row, err := r.Get(ctx, org, id)
	if err != nil {
		return nil, err
	}
	if state != "" && (!validState(state) || !validTransition(row.State, state)) {
		return nil, ErrInvalidTransition
	}
	if name != "" {
		row.Name = strings.TrimSpace(name)
	}
	if brief != "" {
		row.Brief = strings.TrimSpace(brief)
	}
	if state != "" {
		row.State = state
	}
	if row.Name == "" || row.Brief == "" {
		return nil, errors.New("name and brief are required")
	}
	if err := r.db.WithContext(ctx).Where("organization_id = ? AND id = ?", org, id).Save(row).Error; err != nil {
		return nil, fmt.Errorf("update wave: %w", err)
	}
	return row, nil
}
func (r *Repository) Delete(ctx context.Context, org, id string) error {
	res := r.db.WithContext(ctx).Where("organization_id = ? AND id = ?", org, id).Delete(&Wave{})
	if res.Error != nil {
		return fmt.Errorf("delete wave: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
