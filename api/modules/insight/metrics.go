package insight

import (
	"sort"
	"time"
)

func dedup(es []event) []event {
	seen := map[string]bool{}
	out := make([]event, 0, len(es))
	for _, e := range es {
		if e.DeliveryID != "" && seen[e.DeliveryID] {
			continue
		}
		seen[e.DeliveryID] = true
		out = append(out, e)
	}
	return out
}
func rate(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}
func sendBack(es []event) SendBackMetric {
	n, d := 0, 0
	var evidence []Evidence
	last := map[int64]string{}
	for _, e := range dedup(es) {
		if e.issue == nil || e.status == "" {
			continue
		}
		k := *e.issue
		prev := last[k]
		if prev == "in_progress" && e.status == "review" {
			d++
			last[k] = e.status
			continue
		}
		if prev == "review" && e.status == "in_progress" {
			n++
			evidence = append(evidence, Evidence{DeliveryID: e.DeliveryID, EventType: e.EventType, ReceivedAt: e.ReceivedAt, PRNumber: e.pr, IssueNumber: e.issue})
		}
		last[k] = e.status
	}
	return SendBackMetric{Count: n, Rate: rate(n, d), Numerator: n, Denominator: d, Evidence: evidence}
}
func firstCI(es []event) FirstCIMetric {
	groups := map[int64]map[string][]event{}
	for _, e := range dedup(es) {
		if e.pr != nil && e.pipelineID != "" && e.sha != "" && e.conclusion != "" && !e.startedAt.IsZero() && !e.sourceAt.IsZero() {
			if groups[*e.pr] == nil {
				groups[*e.pr] = map[string][]event{}
			}
			groups[*e.pr][e.pipelineID] = append(groups[*e.pr][e.pipelineID], e)
		}
	}
	m := FirstCIMetric{}
	for _, runs := range groups {
		var first event
		for _, items := range runs {
			sort.Slice(items, func(i, j int) bool { return items[i].sourceAt.Before(items[j].sourceAt) })
			if first.pipelineID == "" || items[0].startedAt.Before(first.startedAt) {
				first = items[0]
			}
		}
		e := first
		m.Eligible++
		if e.conclusion == "success" || e.conclusion == "passed" {
			m.Successful++
			m.Evidence = append(m.Evidence, Evidence{DeliveryID: e.DeliveryID, EventType: e.EventType, ReceivedAt: e.ReceivedAt, PRNumber: e.pr, SourceAt: e.sourceAt})
		}
	}
	m.Rate = rate(m.Successful, m.Eligible)
	return m
}
func cycle(es []event) CycleMetric {
	starts := map[int64]time.Time{}
	var vals []float64
	var evidence []Evidence
	for _, e := range dedup(es) {
		if e.issue == nil || e.status == "" {
			continue
		}
		n := *e.issue
		if e.status == "in_progress" && starts[n].IsZero() {
			starts[n] = e.ReceivedAt
		}
		if e.status == "done" && !starts[n].IsZero() {
			vals = append(vals, e.ReceivedAt.Sub(starts[n]).Seconds())
			evidence = append(evidence, Evidence{DeliveryID: e.DeliveryID, EventType: e.EventType, ReceivedAt: e.ReceivedAt, IssueNumber: e.issue})
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
	for _, e := range dedup(es) {
		if (e.conclusion == "failure" || e.conclusion == "failed") && e.sha != "" {
			by[e.sha] = append(by[e.sha], Evidence{DeliveryID: e.DeliveryID, EventType: e.EventType, ReceivedAt: e.ReceivedAt, PRNumber: e.pr})
		}
	}
	out := []Suggestion{}
	for k, v := range by {
		if len(v) >= 2 {
			out = append(out, Suggestion{Key: k, Message: "Repeated CI failure for the same SHA: review recorded evidence before adding a checklist item.", Occurrences: len(v), Evidence: v})
		}
	}
	return out
}
