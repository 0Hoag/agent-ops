package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
)

func (p *Provider) Backfill(ctx context.Context, repository string) (scm.BackfillResult, error) {
	var result scm.BackfillResult
	owner, name, ok := strings.Cut(repository, "/")
	if !ok || owner == "" || name == "" {
		return result, fmt.Errorf("invalid GitHub repository %q", repository)
	}
	repoPath := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name)
	issues, err := list[issueJSON](p, ctx, repoPath+"/issues", true)
	if err != nil {
		return result, fmt.Errorf("list GitHub issues: %w", err)
	}
	for _, raw := range issues {
		if raw.PullRequest == nil {
			result.Issues = append(result.Issues, mapIssue(raw, repository))
		}
	}
	pulls, err := list[pullJSON](p, ctx, repoPath+"/pulls", true)
	if err != nil {
		return result, fmt.Errorf("list GitHub pull requests: %w", err)
	}
	for _, raw := range pulls {
		result.PullRequests = append(result.PullRequests, mapPull(raw, repository))
	}
	runs, err := list[runJSON](p, ctx, repoPath+"/actions/runs", false)
	if err != nil {
		return result, fmt.Errorf("list GitHub workflow runs: %w", err)
	}
	for _, raw := range runs {
		result.PipelineRuns = append(result.PipelineRuns, mapRun(raw, repository))
	}
	return result, nil
}

func list[T any](p *Provider, ctx context.Context, path string, allStates bool) ([]T, error) {
	var result []T
	for page := 1; ; page++ {
		query := "?per_page=100&page=" + strconv.Itoa(page)
		if allStates {
			query += "&state=all"
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+path+query, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Accept", "application/vnd.github+json")
		if p.token != "" {
			request.Header.Set("Authorization", "Bearer "+p.token)
		}
		response, err := p.client.Do(request)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		closeErr := response.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return nil, fmt.Errorf("GitHub API returned %s", response.Status)
		}
		var values []T
		if strings.HasSuffix(path, "/actions/runs") {
			var envelope struct {
				WorkflowRuns []T `json:"workflow_runs"`
			}
			if err := json.Unmarshal(body, &envelope); err != nil {
				return nil, err
			}
			values = envelope.WorkflowRuns
		} else if err := json.Unmarshal(body, &values); err != nil {
			return nil, err
		}
		result = append(result, values...)
		if !hasNext(response.Header.Get("Link")) {
			return result, nil
		}
	}
}

func hasNext(link string) bool {
	for _, part := range strings.Split(link, ",") {
		if strings.Contains(part, `rel="next"`) {
			return true
		}
	}
	return false
}
