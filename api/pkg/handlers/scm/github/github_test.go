package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm/builder"
	"github.com/stretchr/testify/require"
)

func signed(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"raw":"bytes"}`)
	tests := []struct {
		name, signature string
		wantErr         bool
	}{
		{"valid", signed("secret", body), false}, {"wrong secret", signed("other", body), true}, {"altered body", signed("secret", []byte(`{"raw":"changed"}`)), true}, {"absent", "", true}, {"malformed", "sha256=not-hex", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := VerifySignature("secret", body, test.signature)
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestProviderRegistersWithConfiguredRegistry(t *testing.T) {
	provider, err := builder.Build("github", scm.Config{Token: "token", WebhookSecret: "secret"})
	require.NoError(t, err)
	require.NotNil(t, provider)
}

func TestParseEventMappers(t *testing.T) {
	secret := "secret"
	tests := []struct {
		name, kind, fixture string
		check               func(t *testing.T, event scm.Event)
	}{
		{"issue status and close", "issues", "issues.json", func(t *testing.T, e scm.Event) {
			require.Equal(t, scm.StatusDone, e.Issue.Status)
			require.Equal(t, "acme/project", e.Repository)
		}},
		{"pull request refs", "pull_request", "pull_request.json", func(t *testing.T, e scm.Event) {
			require.Equal(t, []int{7}, e.PullRequest.ClosingIssueNumbers)
			require.Equal(t, "abc", e.PullRequest.HeadSHA)
		}},
		{"workflow run", "workflow_run", "workflow_run.json", func(t *testing.T, e scm.Event) {
			require.Equal(t, "success", e.PipelineRun.Conclusion)
			require.Equal(t, 8, *e.PipelineRun.PRNumber)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := os.ReadFile("testdata/" + test.fixture)
			require.NoError(t, err)
			headers := http.Header{"X-Hub-Signature-256": []string{signed(secret, body)}, "X-GitHub-Delivery": []string{"delivery-1"}}
			event, err := New(Config{WebhookSecret: secret}).ParseEvent(context.Background(), test.kind, headers, body)
			require.NoError(t, err)
			test.check(t, event)
		})
	}
}

func TestIssueStatusMapping(t *testing.T) {
	tests := []struct {
		label  string
		status scm.IssueStatus
	}{
		{"status:todo", scm.StatusTodo}, {"status:in_progress", scm.StatusInProgress}, {"status:in progress", scm.StatusInProgress},
		{"status:review", scm.StatusReview}, {"status:blocked", scm.StatusBlocked}, {"status:done", scm.StatusDone},
	}
	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			issue := mapIssue(issueJSON{ID: 1, Labels: []struct {
				Name string `json:"name"`
			}{{Name: test.label}}}, "acme/project")
			require.Equal(t, test.status, issue.Status)
		})
	}
}

func TestBackfillPaginationCancellationAndPRExclusion(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.URL.Path+"?"+request.URL.RawQuery)
		writer.Header().Set("Link", `<http://example.test?page=2>; rel="next"`)
		switch request.URL.Path {
		case "/repos/acme/project/issues":
			if request.URL.Query().Get("page") == "1" {
				_, _ = writer.Write([]byte(`[{"id":1,"number":1,"title":"Issue"},{"id":2,"number":2,"pull_request":{"url":"x"}}]`))
			} else {
				writer.Header().Del("Link")
				_, _ = writer.Write([]byte(`[{"id":3,"number":3,"title":"Issue 2"}]`))
			}
		case "/repos/acme/project/pulls":
			writer.Header().Del("Link")
			_, _ = writer.Write([]byte(`[{"id":4,"number":4,"head":{"sha":"sha"}}]`))
		case "/repos/acme/project/actions/runs":
			writer.Header().Del("Link")
			_, _ = writer.Write([]byte(`{"workflow_runs":[{"id":5,"status":"completed"}]}`))
		}
	}))
	defer server.Close()
	result, err := New(Config{APIBaseURL: server.URL, Token: "token"}).Backfill(context.Background(), "acme/project")
	require.NoError(t, err)
	require.Len(t, result.Issues, 2)
	require.Len(t, result.PullRequests, 1)
	require.Len(t, result.PipelineRuns, 1)
	require.Len(t, requests, 4)
	require.Contains(t, requests[0], "/repos/acme/project/issues?")
	require.Contains(t, requests[0], "state=all")
	require.Contains(t, requests[2], "/repos/acme/project/pulls?")
	require.Contains(t, requests[2], "state=all")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = New(Config{APIBaseURL: server.URL}).Backfill(ctx, "acme/project")
	require.ErrorIs(t, err, context.Canceled)
}

func TestMapRunFinishedAt(t *testing.T) {
	run := mapRun(runJSON{ID: 1, UpdatedAt: time.Now(), Conclusion: "failure"}, "repo")
	require.NotNil(t, run.FinishedAt)
}

func TestMapPullNormalizesMergedState(t *testing.T) {
	mergedAt := time.Now().UTC()
	pr := mapPull(pullJSON{ID: 1, State: "closed", MergedAt: &mergedAt}, "acme/project")
	require.True(t, pr.Merged)
	require.Equal(t, "merged", pr.State)
}

func TestClosingIssuesRequiresClosingKeywordAndSameRepository(t *testing.T) {
	body := "Related to #1; closes #2, fixes acme/project#3, resolves acme/other#4"
	require.Equal(t, []int{2, 3}, closingIssues(body, "acme/project"))
}
