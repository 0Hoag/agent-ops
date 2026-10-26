package scm

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// Config contains provider credentials and transport settings shared by SCM vendors.
type Config struct {
	Token          string
	WebhookSecret  string
	APIBaseURL     string
	HTTPClient     *http.Client
	StatusMappings map[string]string
}

func MapStatus(provider, label string, custom map[string]string) IssueStatus {
	if custom != nil {
		if status, ok := custom[label]; ok {
			return IssueStatus(status)
		}
	}
	return IssueStatus("")
}

func ApplyStatusMapping(provider string, result *BackfillResult, mappings map[string]string) {
	for i := range result.Issues {
		if result.Issues[i].State == "closed" {
			result.Issues[i].Status = StatusDone
			continue
		}
		for _, label := range result.Issues[i].Labels {
			if value, ok := mappings[label]; ok {
				result.Issues[i].Status = IssueStatus(value)
				break
			}
			l := strings.ToLower(strings.TrimSpace(label))
			if strings.HasPrefix(l, "status:") {
				v := strings.TrimPrefix(strings.TrimPrefix(l, "status::"), "status:")
				result.Issues[i].Status = IssueStatus(strings.ReplaceAll(v, " ", "_"))
				break
			}
		}
	}
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
	UpdatedAt  time.Time
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
