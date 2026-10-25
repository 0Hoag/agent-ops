package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
)

type project struct {
	ID                int64  `json:"id"`
	PathWithNamespace string `json:"path_with_namespace"`
}
type issueJSON struct {
	ID          int64             `json:"id"`
	IID         int               `json:"iid"`
	Title       string            `json:"title"`
	State       string            `json:"state"`
	Description string            `json:"description"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Labels      []json.RawMessage `json:"labels"`
	Assignee    *struct {
		Username string `json:"username"`
	} `json:"assignee"`
}
type mrJSON struct {
	ID           int64      `json:"id"`
	IID          int        `json:"iid"`
	Title        string     `json:"title"`
	State        string     `json:"state"`
	Description  string     `json:"description"`
	UpdatedAt    time.Time  `json:"updated_at"`
	SHA          string     `json:"sha"`
	SourceBranch string     `json:"source_branch"`
	MergedAt     *time.Time `json:"merged_at"`
	MergeStatus  string     `json:"merge_status"`
	Author       struct {
		Username string `json:"username"`
	} `json:"author"`
}
type pipelineJSON struct {
	ID           int64      `json:"id"`
	SHA          string     `json:"sha"`
	Status       string     `json:"status"`
	Ref          string     `json:"ref"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	MergeRequest *struct {
		IID int `json:"iid"`
	} `json:"merge_request"`
}
type eventEnvelope struct {
	ObjectAttributes json.RawMessage `json:"object_attributes"`
	Project          project         `json:"project"`
	User             struct {
		Username string `json:"username"`
	} `json:"user"`
	MergeRequest *struct {
		IID int `json:"iid"`
	} `json:"merge_request"`
}

func (p *Provider) ParseEvent(ctx context.Context, eventType string, headers http.Header, payload []byte) (scm.Event, error) {
	if err := p.VerifyWebhook(ctx, headers, payload); err != nil {
		return scm.Event{}, err
	}
	var env eventEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return scm.Event{}, fmt.Errorf("decode GitLab event: %w", err)
	}
	e := scm.Event{DeliveryID: headers.Get("X-Gitlab-Event-UUID"), EventType: eventType, Repository: env.Project.PathWithNamespace, ReceivedAt: time.Now().UTC(), Payload: append([]byte(nil), payload...)}
	if e.Repository == "" {
		return scm.Event{}, fmt.Errorf("incomplete GitLab event")
	}
	switch strings.ToLower(eventType) {
	case "issue hook", "issue":
		var raw issueJSON
		if err := json.Unmarshal(env.ObjectAttributes, &raw); err != nil {
			return scm.Event{}, err
		}
		if raw.ID <= 0 || raw.IID <= 0 || raw.Title == "" {
			return scm.Event{}, fmt.Errorf("incomplete issue event")
		}
		e.Issue = ptrIssue(raw, e.Repository)
	case "merge request hook", "merge_request":
		var raw mrJSON
		if err := json.Unmarshal(env.ObjectAttributes, &raw); err != nil {
			return scm.Event{}, err
		}
		if raw.SHA == "" {
			var details struct {
				LastCommit struct {
					ID string `json:"id"`
				} `json:"last_commit"`
			}
			_ = json.Unmarshal(env.ObjectAttributes, &details)
			raw.SHA = details.LastCommit.ID
		}
		if raw.ID <= 0 || raw.IID <= 0 || raw.Title == "" || raw.SHA == "" {
			return scm.Event{}, fmt.Errorf("incomplete merge request event")
		}
		e.PullRequest = ptrMR(raw, e.Repository)
	case "pipeline hook", "pipeline":
		var raw pipelineJSON
		if err := json.Unmarshal(env.ObjectAttributes, &raw); err != nil {
			return scm.Event{}, err
		}
		if raw.ID <= 0 || raw.SHA == "" || raw.Status == "" {
			return scm.Event{}, fmt.Errorf("incomplete pipeline event")
		}
		e.PipelineRun = ptrPipeline(raw, e.Repository)
		if e.PipelineRun.PRNumber == nil && env.MergeRequest != nil && env.MergeRequest.IID > 0 {
			e.PipelineRun.PRNumber = &env.MergeRequest.IID
		}
	default:
		return scm.Event{}, fmt.Errorf("unsupported GitLab event %q", eventType)
	}
	return e, nil
}

func ptrIssue(r issueJSON, repo string) *scm.Issue {
	status := scm.StatusTodo
	labels := make([]string, 0, len(r.Labels))
	for _, raw := range r.Labels {
		var l string
		if json.Unmarshal(raw, &l) != nil {
			var object struct {
				Title string `json:"title"`
			}
			_ = json.Unmarshal(raw, &object)
			l = object.Title
		}
		if l == "" {
			continue
		}
		labels = append(labels, l)
		if s, ok := mapStatus(l); ok {
			status = s
		}
	}
	if r.State == "closed" {
		status = scm.StatusDone
	}
	a := ""
	if r.Assignee != nil {
		a = r.Assignee.Username
	}
	return &scm.Issue{ExternalID: strconv.FormatInt(r.ID, 10), Number: r.IID, Repository: repo, Title: r.Title, State: r.State, Status: status, Labels: labels, Assignee: a, UpdatedAt: r.UpdatedAt}
}
func mapStatus(label string) (scm.IssueStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "status::todo":
		return scm.StatusTodo, true
	case "status::in progress", "status::in_progress":
		return scm.StatusInProgress, true
	case "status::review":
		return scm.StatusReview, true
	case "status::blocked":
		return scm.StatusBlocked, true
	case "status::done":
		return scm.StatusDone, true
	}
	return "", false
}
func ptrMR(r mrJSON, repo string) *scm.PullRequest {
	state := r.State
	merged := r.MergedAt != nil || state == "merged"
	if merged {
		state = "merged"
	}
	return &scm.PullRequest{ExternalID: strconv.FormatInt(r.ID, 10), Number: r.IID, Repository: repo, Title: r.Title, State: state, HeadSHA: r.SHA, HeadBranch: r.SourceBranch, Merged: merged, MergedAt: r.MergedAt, ClosingIssueNumbers: closingIssues(r.Description, repo), UpdatedAt: r.UpdatedAt}
}
func ptrPipeline(r pipelineJSON, repo string) *scm.PipelineRun {
	var pr *int
	if r.MergeRequest != nil {
		pr = &r.MergeRequest.IID
	}
	var finished *time.Time
	if r.FinishedAt != nil {
		finished = r.FinishedAt
	} else if r.Status == "success" || r.Status == "failed" || r.Status == "canceled" {
		finished = &r.UpdatedAt
	}
	return &scm.PipelineRun{ExternalID: strconv.FormatInt(r.ID, 10), Repository: repo, PRNumber: pr, SHA: r.SHA, Status: r.Status, Conclusion: r.Status, StartedAt: r.CreatedAt, FinishedAt: finished, UpdatedAt: r.UpdatedAt}
}

var refRE = regexp.MustCompile(`(?i)(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+(?:[\w.-]+/[\w.-]+)?#(\d+)`)

func closingIssues(body, _ string) []int {
	var out []int
	for _, m := range refRE.FindAllStringSubmatch(body, -1) {
		n, _ := strconv.Atoi(m[1])
		if n > 0 {
			out = append(out, n)
		}
	}
	return out
}
