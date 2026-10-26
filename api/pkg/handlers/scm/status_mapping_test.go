package scm

import "testing"

func TestApplyStatusMappingUsesRepositoryMappingsAndClosedDone(t *testing.T) {
	result := BackfillResult{Issues: []Issue{
		{Number: 1, State: "open", Labels: []string{"Status::Review"}},
		{Number: 2, State: "open", Labels: []string{"needs-triage"}},
		{Number: 3, State: "closed", Labels: []string{"Status::Blocked"}},
	}}
	ApplyStatusMapping("gitlab", &result, map[string]string{
		"needs-triage":   string(StatusBlocked),
		"Status::Review": string(StatusReview),
	})
	if result.Issues[0].Status != StatusReview {
		t.Fatalf("default GitLab mapping got %q", result.Issues[0].Status)
	}
	if result.Issues[1].Status != StatusBlocked {
		t.Fatalf("custom mapping got %q", result.Issues[1].Status)
	}
	if result.Issues[2].Status != StatusDone {
		t.Fatalf("closed issue got %q, want done", result.Issues[2].Status)
	}
}

func TestApplyStatusMappingSupportsGitHubDefaults(t *testing.T) {
	result := BackfillResult{Issues: []Issue{{State: "open", Labels: []string{"status:in progress"}}}}
	ApplyStatusMapping("github", &result, nil)
	if result.Issues[0].Status != StatusInProgress {
		t.Fatalf("GitHub default mapping got %q", result.Issues[0].Status)
	}
}
