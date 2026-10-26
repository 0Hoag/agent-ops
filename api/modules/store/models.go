package store

import "time"

type Repository struct {
	ID                      UUID   `gorm:"type:uuid;primaryKey"`
	Provider                string `gorm:"not null"`
	FullName                string `gorm:"not null"`
	WebhookSecretEnvRef     string `gorm:"not null"`
	OrganizationID          string `gorm:"type:uuid;not null;index"`
	WebhookSecretCiphertext string `gorm:"not null;default:''"`
	BackfillTokenCiphertext string `gorm:"not null;default:''"`
	GitLabBaseURL           string `gorm:"not null;default:''"`
	StatusMappings          JSONB  `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type Issue struct {
	ID                UUID   `gorm:"type:uuid;primaryKey"`
	RepositoryID      UUID   `gorm:"type:uuid;not null;index"`
	Number            int64  `gorm:"not null"`
	Title             string `gorm:"not null"`
	State             string `gorm:"not null"`
	Status            string `gorm:"not null"`
	Labels            JSONB  `gorm:"type:jsonb;not null"`
	Assignee          string
	UpstreamUpdatedAt *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PullRequest struct {
	ID                UUID   `gorm:"type:uuid;primaryKey"`
	RepositoryID      UUID   `gorm:"type:uuid;not null;index"`
	Number            int64  `gorm:"not null"`
	Title             string `gorm:"not null"`
	State             string `gorm:"not null"`
	HeadSHA           string `gorm:"not null"`
	Branch            string `gorm:"not null"`
	Closes            JSONB  `gorm:"type:jsonb;not null"`
	MergedAt          *time.Time
	UpstreamUpdatedAt *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PipelineRun struct {
	ID                UUID   `gorm:"type:uuid;primaryKey"`
	RepositoryID      UUID   `gorm:"type:uuid;not null;index"`
	ExternalID        string `gorm:"not null"`
	PRNumber          *int64
	SHA               string `gorm:"not null"`
	Status            string `gorm:"not null"`
	Conclusion        string `gorm:"not null"`
	StartedAt         *time.Time
	FinishedAt        *time.Time
	UpstreamUpdatedAt *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type WebhookEvent struct {
	ID           UUID      `gorm:"type:uuid;primaryKey"`
	RepositoryID UUID      `gorm:"type:uuid;not null;index"`
	Provider     string    `gorm:"not null"`
	EventType    string    `gorm:"not null"`
	DeliveryID   string    `gorm:"not null"`
	Payload      JSONB     `gorm:"type:jsonb;not null"`
	ReceivedAt   time.Time `gorm:"not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
