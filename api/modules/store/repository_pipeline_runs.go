package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PipelineRunRepository struct{ db *gorm.DB }

func (repository *PipelineRunRepository) Upsert(ctx context.Context, item *PipelineRun) error {
	err := repository.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "repository_id"}, {Name: "external_id"}}, DoUpdates: clause.Assignments(map[string]any{
		"pr_number": gorm.Expr("COALESCE(EXCLUDED.pr_number, pipeline_runs.pr_number)"), "sha": gorm.Expr("COALESCE(NULLIF(EXCLUDED.sha, ''), pipeline_runs.sha)"), "status": gorm.Expr("COALESCE(NULLIF(EXCLUDED.status, ''), pipeline_runs.status)"), "conclusion": gorm.Expr("COALESCE(NULLIF(EXCLUDED.conclusion, ''), pipeline_runs.conclusion)"), "started_at": gorm.Expr("COALESCE(EXCLUDED.started_at, pipeline_runs.started_at)"), "finished_at": gorm.Expr("COALESCE(EXCLUDED.finished_at, pipeline_runs.finished_at)"), "updated_at": gorm.Expr("NOW()"),
	})}).Create(item).Error
	if err != nil {
		return fmt.Errorf("upsert pipeline run %s: %w", item.ExternalID, err)
	}
	return nil
}

func (repository *PipelineRunRepository) GetByExternalID(ctx context.Context, repositoryID UUID, externalID string) (*PipelineRun, error) {
	var item PipelineRun
	if err := repository.db.WithContext(ctx).Where("repository_id = ? AND external_id = ?", repositoryID, externalID).First(&item).Error; err != nil {
		return nil, fmt.Errorf("get pipeline run %s: %w", externalID, err)
	}
	return &item, nil
}

func (repository *PipelineRunRepository) ListBySHA(ctx context.Context, repositoryID UUID, sha string) ([]PipelineRun, error) {
	var items []PipelineRun
	if err := repository.db.WithContext(ctx).Where("repository_id = ? AND sha = ?", repositoryID, sha).Order("started_at DESC NULLS LAST").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list pipeline runs for sha %s: %w", sha, err)
	}
	return items, nil
}
