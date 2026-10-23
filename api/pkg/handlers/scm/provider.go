package scm

import (
	"context"
	"net/http"
	"time"
)

// Config contains provider credentials and transport settings shared by SCM vendors.
type Config struct {
	Token         string
	WebhookSecret string
	APIBaseURL    string
	HTTPClient    *http.Client
}

// IssueStatus is the provider-neutral board status.
type IssueStatus string

const (
	StatusTodo       IssueStatus = "todo"
	StatusInProgress IssueStatus = "in_progress"
	StatusReview     IssueStatus = "review"
	StatusBlocked    IssueStatus = "blocked"
	StatusDone       IssueStatus = "done"
)

type Issue struct {
	ExternalID string
	Number     int
	Repository string
	Title      string
	State      string
	Status     IssueStatus
	Labels     []string
	Assignee   string
	UpdatedAt  time.Time
}

type PullRequest struct {
	ExternalID          string
	Number              int
	Repository          string
	Title               string
	State               string
	HeadSHA             string
	HeadBranch          string
	Merged              bool
	MergedAt            *time.Time
	ClosingIssueNumbers []int
	UpdatedAt           time.Time
}

type PipelineRun struct {
	ExternalID string
	Repository string
	PRNumber   *int
	SHA        string
	Status     string
	Conclusion string
	StartedAt  time.Time
	FinishedAt *time.Time
}

// Event contains normalized data and the delivery metadata needed for idempotency.
type Event struct {
	DeliveryID  string
	EventType   string
	Repository  string
	ReceivedAt  time.Time
	Payload     []byte
	Issue       *Issue
	PullRequest *PullRequest
	PipelineRun *PipelineRun
}

type BackfillResult struct {
	Issues       []Issue
	PullRequests []PullRequest
	PipelineRuns []PipelineRun
}

// SCMProvider is the vendor-neutral boundary used by webhook ingestion and sync.
type SCMProvider interface {
	VerifyWebhook(ctx context.Context, headers http.Header, payload []byte) error
	ParseEvent(ctx context.Context, eventType string, headers http.Header, payload []byte) (Event, error)
	Backfill(ctx context.Context, repository string) (BackfillResult, error)
}
