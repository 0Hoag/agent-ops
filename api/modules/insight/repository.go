package insight

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

type event struct {
	store.WebhookEvent
	status     string
	issue      *int64
	pr         *int64
	sha        string
	conclusion string
}

func (r *Repository) Build(ctx context.Context, id string) (Response, error) {
	var exists int64
	if err := r.db.WithContext(ctx).Model(&store.Repository{}).Where("id = ?", id).Count(&exists).Error; err != nil {
		return Response{}, fmt.Errorf("find repository: %w", err)
	}
	if exists == 0 {
		return Response{}, gorm.ErrRecordNotFound
	}
	var rows []store.WebhookEvent
	if err := r.db.WithContext(ctx).Where("repository_id = ?", id).Order("received_at ASC").Find(&rows).Error; err != nil {
		return Response{}, fmt.Errorf("list events: %w", err)
	}
	es := make([]event, 0, len(rows))
	for _, row := range rows {
		es = append(es, decode(row))
	}
	out := Response{RepositoryID: id, History: History{Complete: len(rows) > 0, Reason: "raw webhook history is retained only from first recorded delivery"}}
	out.SendBack = sendBack(es)
	out.FirstCI = firstCI(es)
	out.CycleTime = cycle(es)
	out.Suggestions = suggestions(es)
	if len(rows) == 0 {
		out.History = History{Reason: "no retained webhook history"}
	}
	return out, nil
}

func decode(row store.WebhookEvent) event {
	var p map[string]any
	_ = json.Unmarshal(row.Payload, &p)
	e := event{WebhookEvent: row}
	e.status = stringValue(p, "status")
	if e.status == "" {
		if l, ok := p["labels"].([]any); ok {
			for _, x := range l {
				if s, ok := x.(string); ok && strings.HasPrefix(s, "status:") {
					e.status = strings.TrimPrefix(s, "status:")
				}
			}
		}
	}
	e.sha = stringValue(p, "sha")
	e.conclusion = stringValue(p, "conclusion")
	e.issue = intValue(p, "issue_number")
	e.pr = intValue(p, "pr_number")
	return e
}
func stringValue(p map[string]any, k string) string {
	if v, ok := p[k].(string); ok {
		return v
	}
	return ""
}
func intValue(p map[string]any, k string) *int64 {
	if v, ok := p[k].(float64); ok {
		n := int64(v)
		return &n
	}
	return nil
}
func rate(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}
func sendBack(es []event) SendBackMetric {
	n, d := 0, 0
	var ev []Evidence
	last := map[int64]string{}
	seen := map[string]bool{}
	for _, e := range es {
		if seen[e.DeliveryID] {
			continue
		}
		seen[e.DeliveryID] = true
		if e.issue == nil || e.status == "" {
			continue
		}
		prev := last[*e.issue]
		if e.status == "review" {
			d++
			if prev == "in_progress" {
				n++
				ev = append(ev, Evidence{e.DeliveryID, e.EventType, e.ReceivedAt, e.pr, e.issue})
			}
		}
		last[*e.issue] = e.status
	}
	return SendBackMetric{Count: n, Rate: rate(n, d), Numerator: n, Denominator: d, Evidence: ev}
}
func firstCI(es []event) FirstCIMetric {
	groups := map[string][]event{}
	for _, e := range es {
		if e.sha != "" {
			groups[e.sha] = append(groups[e.sha], e)
		}
	}
	m := FirstCIMetric{}
	for sha, items := range groups {
		_ = sha
		sort.Slice(items, func(i, j int) bool { return items[i].ReceivedAt.Before(items[j].ReceivedAt) })
		eligible := false
		for _, e := range items {
			if e.conclusion != "" {
				eligible = true
				m.Eligible++
				if e.conclusion == "success" || e.conclusion == "passed" {
					m.Successful++
					m.Evidence = append(m.Evidence, Evidence{e.DeliveryID, e.EventType, e.ReceivedAt, e.pr, e.issue})
				}
				break
			}
		}
		_ = eligible
	}
	m.Rate = rate(m.Successful, m.Eligible)
	return m
}
func cycle(es []event) CycleMetric {
	starts := map[int64]time.Time{}
	var vals []float64
	var evidence []Evidence
	for _, e := range es {
		if e.issue == nil || e.status == "" {
			continue
		}
		n := *e.issue
		if e.status == "in_progress" && starts[n].IsZero() {
			starts[n] = e.ReceivedAt
		}
		if e.status == "done" && !starts[n].IsZero() {
			vals = append(vals, e.ReceivedAt.Sub(starts[n]).Seconds())
			evidence = append(evidence, Evidence{e.DeliveryID, e.EventType, e.ReceivedAt, e.pr, e.issue})
			delete(starts, n)
		}
	}
	sort.Float64s(vals)
	m := CycleMetric{Completed: len(vals), Evidence: evidence}
	if len(vals) > 0 {
		m.MedianSeconds = vals[len(vals)/2]
		if len(vals)%2 == 0 {
			m.MedianSeconds = (vals[len(vals)/2-1] + vals[len(vals)/2]) / 2
		}
	}
	return m
}
func suggestions(es []event) []Suggestion {
	by := map[string][]Evidence{}
	for _, e := range es {
		if e.conclusion == "failure" || e.conclusion == "failed" {
			key := e.sha
			if key == "" {
				key = "unknown"
			}
			by[key] = append(by[key], Evidence{e.DeliveryID, e.EventType, e.ReceivedAt, e.pr, e.issue})
		}
	}
	out := []Suggestion{}
	for k, v := range by {
		if len(v) >= 2 {
			out = append(out, Suggestion{Key: k, Message: "Repeated CI failure: review the recorded evidence before adding a checklist item.", Occurrences: len(v), Evidence: v})
		}
	}
	return out
}
