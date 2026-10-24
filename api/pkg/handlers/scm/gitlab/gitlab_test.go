package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm/builder"
	"github.com/stretchr/testify/require"
)

func TestVerifyTokenAndRegistry(t *testing.T) {
	for _, test := range []struct {
		name, token string
		wantErr     bool
	}{{"valid", "secret", false}, {"invalid", "wrong", true}, {"missing", "", true}} {
		t.Run(test.name, func(t *testing.T) {
			err := VerifyToken("secret", test.token)
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
	p, err := builder.Build("gitlab", scm.Config{Token: "t", WebhookSecret: "s"})
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestParseEventsAndStatuses(t *testing.T) {
	for _, test := range []struct {
		label string
		want  scm.IssueStatus
	}{{"Status::Todo", scm.StatusTodo}, {"Status::In Progress", scm.StatusInProgress}, {"Status::In_Progress", scm.StatusInProgress}, {"Status::Review", scm.StatusReview}, {"Status::Blocked", scm.StatusBlocked}, {"Status::Done", scm.StatusDone}} {
		body := `{"object_attributes":{"id":1,"iid":2,"title":"issue","state":"opened","updated_at":"2026-01-01T00:00:00Z","labels":["` + test.label + `"]},"project":{"path_with_namespace":"group/project"}}`
		e, err := New(Config{WebhookSecret: "secret"}).ParseEvent(context.Background(), "Issue Hook", http.Header{"X-Gitlab-Token": []string{"secret"}}, []byte(body))
		require.NoError(t, err)
		require.Equal(t, test.want, e.Issue.Status, test.label)
	}
	mr := `{"object_attributes":{"id":4,"iid":7,"title":"mr","state":"opened","sha":"abc","source_branch":"feature","description":"closes #9 and fixes group/project#10","updated_at":"2026-01-01T00:00:00Z"},"project":{"path_with_namespace":"group/project"}}`
	e, err := New(Config{WebhookSecret: "secret"}).ParseEvent(context.Background(), "Merge Request Hook", http.Header{"X-Gitlab-Token": []string{"secret"}}, []byte(mr))
	require.NoError(t, err)
	require.Equal(t, "abc", e.PullRequest.HeadSHA)
	require.Equal(t, []int{9, 10}, e.PullRequest.ClosingIssueNumbers)
	for _, state := range []string{"opened", "updated", "merged", "closed"} {
		payload := `{"object_attributes":{"id":4,"iid":7,"title":"mr","state":"` + state + `","sha":"abc","updated_at":"2026-01-01T00:00:00Z"},"project":{"path_with_namespace":"group/project"}}`
		_, err := New(Config{WebhookSecret: "secret"}).ParseEvent(context.Background(), "Merge Request Hook", http.Header{"X-Gitlab-Token": []string{"secret"}}, []byte(payload))
		require.NoError(t, err)
	}
	for _, status := range []string{"running", "success", "failed", "canceled"} {
		payload := `{"object_attributes":{"id":5,"sha":"def","status":"` + status + `","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T01:00:00Z","merge_request":{"iid":7}},"project":{"path_with_namespace":"group/project"}}`
		e, err := New(Config{WebhookSecret: "secret"}).ParseEvent(context.Background(), "Pipeline Hook", http.Header{"X-Gitlab-Token": []string{"secret"}}, []byte(payload))
		require.NoError(t, err)
		require.Equal(t, status, e.PipelineRun.Status)
		require.Equal(t, 7, *e.PipelineRun.PRNumber)
	}
}

func TestBackfillHTTPContractPaginationAndCancellation(t *testing.T) {
	var requests []string
	var pages = map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.RequestURI)
		require.Equal(t, "token", r.Header.Get("PRIVATE-TOKEN"))
		page := r.URL.Query().Get("page")
		pages[r.URL.Path+"?"+r.URL.RawQuery]++
		if page == "1" {
			w.Header().Set("X-Next-Page", "2")
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/issues"):
			value := `[{"id":1,"iid":1,"title":"old","state":"closed","updated_at":"2026-01-01T00:00:00Z"}]`
			if page == "2" {
				value = `[{"id":2,"iid":2,"title":"current","state":"opened","labels":["Status::Review"],"updated_at":"2026-01-02T00:00:00Z"}]`
			}
			_, _ = w.Write([]byte(value))
		case strings.HasSuffix(r.URL.Path, "/merge_requests"):
			value := `[{"id":3,"iid":3,"title":"mr","state":"merged","sha":"sha","updated_at":"2026-01-01T00:00:00Z"}]`
			if page == "2" {
				value = `[{"id":4,"iid":4,"title":"closed mr","state":"closed","sha":"sha2","updated_at":"2026-01-02T00:00:00Z"}]`
			}
			_, _ = w.Write([]byte(value))
		default:
			value := `[{"id":5,"sha":"sha","status":"success","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`
			if page == "2" {
				value = `[{"id":6,"sha":"sha2","status":"failed","created_at":"2026-01-02T00:00:00Z","updated_at":"2026-01-02T01:00:00Z","merge_request":{"iid":4}}]`
			}
			_, _ = w.Write([]byte(value))
		}
	}))
	defer server.Close()
	p := New(Config{APIBaseURL: server.URL, Token: "token"})
	got, err := p.Backfill(context.Background(), "group/project")
	require.NoError(t, err)
	require.Equal(t, []string{
		"/api/v4/projects/group%2Fproject/issues?per_page=100&page=1&state=all",
		"/api/v4/projects/group%2Fproject/issues?per_page=100&page=2&state=all",
		"/api/v4/projects/group%2Fproject/merge_requests?per_page=100&page=1&state=all",
		"/api/v4/projects/group%2Fproject/merge_requests?per_page=100&page=2&state=all",
		"/api/v4/projects/group%2Fproject/pipelines?per_page=100&page=1",
		"/api/v4/projects/group%2Fproject/pipelines?per_page=100&page=2",
	}, requests)
	require.Equal(t, 2, len(got.Issues))
	require.Equal(t, 2, len(got.PullRequests))
	require.Equal(t, 2, len(got.PipelineRuns))
	require.Equal(t, scm.StatusDone, got.Issues[0].Status)
	require.Equal(t, scm.StatusReview, got.Issues[1].Status)
	require.Equal(t, "closed", got.PullRequests[1].State)
	require.Equal(t, 4, *got.PipelineRuns[1].PRNumber)
	require.Equal(t, "failed", got.PipelineRuns[1].Conclusion)
	require.NotNil(t, got.PipelineRuns[1].FinishedAt)
	first, err := json.Marshal(got)
	require.NoError(t, err)
	gotAgain, err := p.Backfill(context.Background(), "group/project")
	require.NoError(t, err)
	second, err := json.Marshal(gotAgain)
	require.NoError(t, err)
	require.JSONEq(t, string(first), string(second))
	for key, count := range pages {
		require.Equal(t, 2, count, key)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = p.Backfill(ctx, "group/project")
	require.ErrorIs(t, err, context.Canceled)
}

func TestBackfillReportsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/issues") {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`["not an object"]`))
	}))
	defer server.Close()
	_, err := New(Config{APIBaseURL: server.URL}).Backfill(context.Background(), "group/project")
	require.Error(t, err)
}
