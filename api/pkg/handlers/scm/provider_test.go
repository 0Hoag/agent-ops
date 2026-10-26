package scm

import "testing"

func TestApplyStatusMappingHonorsCustomLabelsAndClosedDone(t *testing.T) {
	r := BackfillResult{Issues: []Issue{{State: "open", Labels: []string{"workflow:ready"}}, {State: "closed", Labels: []string{"workflow:ready"}}}}
	ApplyStatusMapping("github", &r, map[string]string{"workflow:ready": string(StatusReview)})
	if r.Issues[0].Status != StatusReview || r.Issues[1].Status != StatusDone {
		t.Fatalf("unexpected statuses: %#v", r.Issues)
	}
}
