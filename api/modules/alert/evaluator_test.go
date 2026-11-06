package alert

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEvaluateFiresBoundariesAndDeduplicates(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	e := NewEvaluator(Thresholds{MinCacheHit: .5, SilentAfter: time.Hour, PRStaleAfter: 24 * time.Hour})
	o := Observation{OrganizationID: "org-a", Repository: "repo-a", Runs: []RunObservation{{Worker: "w", InputTokens: 51, CachedInputTokens: 49, StartedAt: now.Add(-2 * time.Hour), LastActivityAt: now.Add(-time.Hour - time.Minute), Active: true}}, Pipelines: []PipelineObservation{{ID: "ci", Branch: "main", Status: "completed", Conclusion: "failure"}}, PullRequests: []PullRequestObservation{{Number: 7, State: "open", UpdatedAt: now.Add(-25 * time.Hour)}}}
	got := e.Evaluate(context.Background(), now, o)
	require.Len(t, got, 4)
	require.Empty(t, e.Evaluate(context.Background(), now, o))
	got = e.Evaluate(context.Background(), now, Observation{OrganizationID: "org-b", Repository: "repo-a", Runs: o.Runs})
	require.Len(t, got, 2)
	e.Evaluate(context.Background(), now, Observation{OrganizationID: "org-a", Repository: "repo-a", Runs: []RunObservation{{Worker: "w", InputTokens: 50, CachedInputTokens: 50, StartedAt: now, LastActivityAt: now, Active: true}}})
	got = e.Evaluate(context.Background(), now, Observation{OrganizationID: "org-a", Repository: "repo-a", Runs: []RunObservation{{Worker: "w", InputTokens: 90, CachedInputTokens: 10, StartedAt: now, LastActivityAt: now, Active: true}}})
	require.Len(t, got, 1)
}

func TestEvaluateDoesNotFireNonQualifyingInputs(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	e := NewEvaluator(Thresholds{MinCacheHit: .5, SilentAfter: time.Hour, PRStaleAfter: 24 * time.Hour})
	o := Observation{Runs: []RunObservation{{Worker: "zero", InputTokens: 0, CachedInputTokens: 0, Active: true}, {Worker: "idle", LastActivityAt: now.Add(-2 * time.Hour), Active: false}, {Worker: "fresh", InputTokens: 50, CachedInputTokens: 50, LastActivityAt: now, Active: true}}, Pipelines: []PipelineObservation{{ID: "feature", Branch: "feature", Status: "completed", Conclusion: "failure"}, {ID: "pending", Branch: "main", Status: "pending", Conclusion: ""}, {ID: "ok", Branch: "main", Status: "completed", Conclusion: "success"}}, PullRequests: []PullRequestObservation{{Number: 1, State: "closed", UpdatedAt: now.Add(-48 * time.Hour)}, {Number: 2, State: "open", Merged: true, UpdatedAt: now.Add(-48 * time.Hour)}, {Number: 3, State: "open", UpdatedAt: now.Add(-24 * time.Hour)}}}
	require.Empty(t, e.Evaluate(context.Background(), now, o))
}
