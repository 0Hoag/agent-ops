package insight

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type event struct {
	store.WebhookEvent
	status, sha, conclusion string
	pipelineID              string
	issue, pr               *int64
	startedAt, sourceAt     time.Time
}

func (r *Repository) Build(ctx context.Context, id string) (Response, error) {
	var n int64
	q := r.db.WithContext(ctx).Model(&store.Repository{}).Where("id = ?", id)
	if organization := organizationFromContext(ctx); organization != "" {
		q = q.Where("organization_id = ?", organization)
	}
	if err := q.Count(&n).Error; err != nil {
		return Response{}, fmt.Errorf("find repository: %w", err)
	}
	if n == 0 {
		return Response{}, gorm.ErrRecordNotFound
	}
	var rows []store.WebhookEvent
	events := r.db.WithContext(ctx).Where("repository_id = ?", id)
	if organization := organizationFromContext(ctx); organization != "" {
		events = events.Where("repository_id IN (SELECT id FROM repositories WHERE organization_id = ?)", organization)
	}
	if err := events.Order("received_at ASC").Find(&rows).Error; err != nil {
		return Response{}, fmt.Errorf("list events: %w", err)
	}
	es := make([]event, 0, len(rows))
	for _, row := range rows {
		es = append(es, decode(row))
	}
	out := Response{RepositoryID: id, History: History{Reason: "retained history has no known backfill boundary; completeness is unknown"}}
	out.SendBack = sendBack(es)
	out.FirstCI = firstCI(es)
	out.CycleTime = cycle(es)
	out.Suggestions = suggestions(es)
	if len(rows) == 0 {
		out.History.Reason = "no retained webhook history"
	}
	return out, nil
}

type issuePayload struct {
	Issue struct {
		Number int64  `json:"number"`
		State  string `json:"state"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	} `json:"issue"`
}
type labelPayload struct {
	Name  string `json:"name"`
	Title string `json:"title"`
}
type runPayload struct {
	Run struct {
		ID         *int64    `json:"id"`
		HeadSHA    string    `json:"head_sha"`
		Conclusion string    `json:"conclusion"`
		Started    time.Time `json:"run_started_at"`
		Updated    time.Time `json:"updated_at"`
		PRs        []struct {
			Number int64 `json:"number"`
		} `json:"pull_requests"`
	} `json:"workflow_run"`
}
type gitlabPayload struct {
	Object struct {
		IID      int64     `json:"iid"`
		ID       *int64    `json:"id"`
		State    string    `json:"state"`
		SHA      string    `json:"sha"`
		Status   string    `json:"status"`
		Created  time.Time `json:"created_at"`
		Started  time.Time `json:"started_at"`
		Finished time.Time `json:"finished_at"`
		Updated  time.Time `json:"updated_at"`
		MR       *struct {
			IID int64 `json:"iid"`
		} `json:"merge_request"`
		Labels []json.RawMessage `json:"labels"`
	} `json:"object_attributes"`
	Labels []json.RawMessage `json:"labels"`
	MR     *struct {
		IID int64 `json:"iid"`
	} `json:"merge_request"`
}

func decode(row store.WebhookEvent) event {
	e := event{WebhookEvent: row, sourceAt: row.ReceivedAt}
	switch strings.ToLower(row.Provider) {
	case "github":
		if strings.EqualFold(row.EventType, "issues") {
			var p issuePayload
			_ = json.Unmarshal(row.Payload, &p)
			n := p.Issue.Number
			if n != 0 {
				e.issue = &n
			}
			e.status = p.Issue.State
			statusFound := false
			for _, l := range p.Issue.Labels {
				if strings.HasPrefix(l.Name, "status:") {
					e.status = normalizeStatus(strings.TrimPrefix(l.Name, "status:"))
					statusFound = true
				}
			}
			if p.Issue.State == "closed" && !statusFound {
				e.status = "done"
			}
		}
		if strings.EqualFold(row.EventType, "workflow_run") {
			var p runPayload
			_ = json.Unmarshal(row.Payload, &p)
			e.sha = p.Run.HeadSHA
			if p.Run.ID != nil && *p.Run.ID > 0 {
				e.pipelineID = fmt.Sprint(*p.Run.ID)
			}
			e.conclusion = p.Run.Conclusion
			e.startedAt = p.Run.Started
			if !p.Run.Updated.IsZero() {
				e.sourceAt = p.Run.Updated
			}
			if len(p.Run.PRs) > 0 {
				n := p.Run.PRs[0].Number
				e.pr = &n
			}
		}
		if strings.EqualFold(row.EventType, "pull_request") {
			var p struct {
				PR struct {
					Number int64 `json:"number"`
				} `json:"pull_request"`
			}
			_ = json.Unmarshal(row.Payload, &p)
			n := p.PR.Number
			if n != 0 {
				e.pr = &n
			}
		}
	case "gitlab":
		var p gitlabPayload
		_ = json.Unmarshal(row.Payload, &p)
		a := p.Object
		if strings.Contains(strings.ToLower(row.EventType), "pipeline") {
			e.sha = a.SHA
			if a.ID != nil && *a.ID > 0 {
				e.pipelineID = fmt.Sprint(*a.ID)
			}
			e.conclusion = a.Status
			e.startedAt = a.Started
			if e.startedAt.IsZero() {
				e.startedAt = a.Created
			}
			e.sourceAt = a.Finished
			if e.sourceAt.IsZero() {
				e.sourceAt = a.Updated
			}
			if a.MR != nil {
				n := a.MR.IID
				e.pr = &n
			} else if p.MR != nil {
				n := p.MR.IID
				e.pr = &n
			}
		} else {
			n := a.IID
			if n != 0 {
				e.issue = &n
			}
			if !strings.Contains(strings.ToLower(row.EventType), "issue") || strings.Contains(strings.ToLower(row.EventType), "merge request") {
				break
			}
			labels := decodeGitLabLabels(a.Labels)
			if len(labels) == 0 {
				labels = decodeGitLabLabels(p.Labels)
			}
			e.status = gitlabIssueStatus(a.State, labels)
		}
	}
	return e
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "in progress", "in_progress":
		return "in_progress"
	case "todo":
		return "todo"
	case "review", "blocked", "done":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return ""
	}
}

func gitlabIssueStatus(state string, labels []labelPayload) string {
	if state == "closed" {
		return "done"
	}
	for _, label := range labels {
		name := label.Title
		if name == "" {
			name = label.Name
		}
		name = strings.ToLower(strings.TrimSpace(name))
		if strings.HasPrefix(name, "status::") {
			if status := normalizeStatus(strings.TrimPrefix(name, "status::")); status != "" {
				return status
			}
		}
	}
	return ""
}

func decodeGitLabLabels(rawLabels []json.RawMessage) []labelPayload {
	labels := make([]labelPayload, 0, len(rawLabels))
	for _, raw := range rawLabels {
		var label labelPayload
		if json.Unmarshal(raw, &label) == nil && (label.Title != "" || label.Name != "") {
			labels = append(labels, label)
			continue
		}
		var name string
		if json.Unmarshal(raw, &name) == nil {
			labels = append(labels, labelPayload{Title: name})
		}
	}
	return labels
}
