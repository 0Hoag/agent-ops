package store

import "gorm.io/gorm"

type Store struct {
	db            *gorm.DB
	Repositories  *RepositoryRepository
	Issues        *IssueRepository
	PullRequests  *PullRequestRepository
	PipelineRuns  *PipelineRunRepository
	WebhookEvents *WebhookEventRepository
}

func New(db *gorm.DB) *Store {
	return newStore(db)
}

func newStore(db *gorm.DB) *Store {
	return &Store{db: db, Repositories: &RepositoryRepository{db: db}, Issues: &IssueRepository{db: db}, PullRequests: &PullRequestRepository{db: db}, PipelineRuns: &PipelineRunRepository{db: db}, WebhookEvents: &WebhookEventRepository{db: db}}
}

// WithTx returns store repositories bound to an existing transaction.
func (store *Store) WithTx(tx *gorm.DB) *Store { return newStore(tx) }
