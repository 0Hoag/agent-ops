package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
)

type issueJSON struct {
	ID        int64     `json:"id"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	UpdatedAt time.Time `json:"updated_at"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Assignee *struct {
		Login string `json:"login"`
	} `json:"assignee"`
	PullRequest *struct {
		URL string `json:"url"`
	} `json:"pull_request"`
}
type pullJSON struct {
	ID        int64      `json:"id"`
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	Body      string     `json:"body"`
	UpdatedAt time.Time  `json:"updated_at"`
	Merged    bool       `json:"merged"`
	MergedAt  *time.Time `json:"merged_at"`
	Head      struct {
		SHA string `json:"sha"`
		Ref string `json:"ref"`
	} `json:"head"`
}
type runJSON struct {
	ID         int64     `json:"id"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	SHA        string    `json:"head_sha"`
	RunStarted time.Time `json:"run_started_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	PRs        []struct {
		Number int `json:"number"`
	} `json:"pull_requests"`
}
type repositoryJSON struct {
	FullName string `json:"full_name"`
}
type issueEvent struct {
	Issue      issueJSON      `json:"issue"`
	Repository repositoryJSON `json:"repository"`
}
type pullEvent struct {
	Action      string         `json:"action"`
	PullRequest pullJSON       `json:"pull_request"`
	Repository  repositoryJSON `json:"repository"`
}
type runEvent struct {
	Action      string         `json:"action"`
	WorkflowRun runJSON        `json:"workflow_run"`
	Repository  repositoryJSON `json:"repository"`
}

func (p *Provider) ParseEvent(ctx context.Context, eventType string, headers http.Header, payload []byte) (scm.Event, error) {
	if err := p.VerifyWebhook(ctx, headers, payload); err != nil {
		return scm.Event{}, err
	}
	event := scm.Event{DeliveryID: headers.Get("X-GitHub-Delivery"), EventType: eventType, ReceivedAt: time.Now().UTC(), Payload: append([]byte(nil), payload...)}
	switch eventType {
	case "issues":
		var raw issueEvent
		if err := json.Unmarshal(payload, &raw); err != nil {
			return scm.Event{}, fmt.Errorf("decode issues event: %w", err)
		}
		if raw.Issue.ID <= 0 || raw.Issue.Number <= 0 || raw.Issue.Title == "" || (raw.Issue.State != "open" && raw.Issue.State != "closed") || raw.Repository.FullName == "" {
			return scm.Event{}, fmt.Errorf("incomplete issue event")
		}
		issue := mapIssue(raw.Issue, raw.Repository.FullName)
		event.Repository, event.Issue = raw.Repository.FullName, &issue
	case "pull_request":
		var raw pullEvent
		if err := json.Unmarshal(payload, &raw); err != nil {
			return scm.Event{}, fmt.Errorf("decode pull_request event: %w", err)
		}
		if raw.PullRequest.ID <= 0 || raw.PullRequest.Number <= 0 || raw.PullRequest.Title == "" || raw.PullRequest.Head.SHA == "" || raw.PullRequest.Head.Ref == "" || (raw.PullRequest.State != "open" && raw.PullRequest.State != "closed") || raw.Repository.FullName == "" {
			return scm.Event{}, fmt.Errorf("incomplete pull request event")
		}
		pr := mapPull(raw.PullRequest, raw.Repository.FullName)
		pr.State = eventState(raw.Action, raw.PullRequest.State, raw.PullRequest.Merged)
		event.Repository, event.PullRequest = raw.Repository.FullName, &pr
	case "workflow_run":
		var raw runEvent
		if err := json.Unmarshal(payload, &raw); err != nil {
			return scm.Event{}, fmt.Errorf("decode workflow_run event: %w", err)
		}
		if raw.WorkflowRun.ID <= 0 || raw.WorkflowRun.SHA == "" || raw.WorkflowRun.Status == "" || raw.WorkflowRun.RunStarted.IsZero() || raw.Repository.FullName == "" {
			return scm.Event{}, fmt.Errorf("incomplete workflow run event")
		}
		run := mapRun(raw.WorkflowRun, raw.Repository.FullName)
		event.Repository, event.PipelineRun = raw.Repository.FullName, &run
	default:
		return scm.Event{}, fmt.Errorf("unsupported GitHub event %q", eventType)
	}
	return event, nil
}

func mapIssue(raw issueJSON, repo string) scm.Issue {
	labels := make([]string, 0, len(raw.Labels))
	status := scm.StatusTodo
	for _, label := range raw.Labels {
		labels = append(labels, label.Name)
		if mapped, ok := mapStatus(label.Name); ok {
			status = mapped
		}
	}
	if raw.State == "closed" {
		status = scm.StatusDone
	}
	assignee := ""
	if raw.Assignee != nil {
		assignee = raw.Assignee.Login
	}
	return scm.Issue{ExternalID: strconv.FormatInt(raw.ID, 10), Number: raw.Number, Repository: repo, Title: raw.Title, State: raw.State, Status: status, Labels: labels, Assignee: assignee, UpdatedAt: raw.UpdatedAt}
}
func mapStatus(label string) (scm.IssueStatus, bool) {
	if !strings.HasPrefix(label, "status:") {
		return "", false
	}
	switch strings.TrimPrefix(label, "status:") {
	case "todo":
		return scm.StatusTodo, true
	case "in_progress":
		return scm.StatusInProgress, true
	case "in progress":
		return scm.StatusInProgress, true
	case "review":
		return scm.StatusReview, true
	case "blocked":
		return scm.StatusBlocked, true
	case "done":
		return scm.StatusDone, true
	}
	return "", false
}
func eventState(action, state string, merged bool) string {
	if action == "synchronize" {
		return "open"
	}
	if merged {
		return "merged"
	}
	return state
}
func mapPull(raw pullJSON, repo string) scm.PullRequest {
	merged := raw.Merged || raw.MergedAt != nil || raw.State == "merged"
	state := raw.State
	if merged {
		state = "merged"
	}
	return scm.PullRequest{ExternalID: strconv.FormatInt(raw.ID, 10), Number: raw.Number, Repository: repo, Title: raw.Title, State: state, HeadSHA: raw.Head.SHA, HeadBranch: raw.Head.Ref, Merged: merged, MergedAt: raw.MergedAt, ClosingIssueNumbers: closingIssues(raw.Body, repo), UpdatedAt: raw.UpdatedAt}
}
func mapRun(raw runJSON, repo string) scm.PipelineRun {
	var pr *int
	if len(raw.PRs) > 0 {
		pr = &raw.PRs[0].Number
	}
	var finished *time.Time
	if raw.Conclusion != "" {
		finished = &raw.UpdatedAt
	}
	return scm.PipelineRun{ExternalID: strconv.FormatInt(raw.ID, 10), Repository: repo, PRNumber: pr, SHA: raw.SHA, Status: raw.Status, Conclusion: raw.Conclusion, StartedAt: raw.RunStarted, FinishedAt: finished, UpdatedAt: raw.UpdatedAt}
}
