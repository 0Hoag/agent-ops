package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm"
)

func (p *Provider) Backfill(ctx context.Context, repository string) (scm.BackfillResult, error) {
	var out scm.BackfillResult
	path := "/api/v4/projects/" + url.PathEscape(repository)
	issues, err := list[issueJSON](p, ctx, path+"/issues", "state=all")
	if err != nil {
		return out, fmt.Errorf("list GitLab issues: %w", err)
	}
	for _, r := range issues {
		out.Issues = append(out.Issues, *ptrIssue(r, repository))
	}
	mrs, err := list[mrJSON](p, ctx, path+"/merge_requests", "state=all")
	if err != nil {
		return out, fmt.Errorf("list GitLab merge requests: %w", err)
	}
	for _, r := range mrs {
		out.PullRequests = append(out.PullRequests, *ptrMR(r, repository))
	}
	pipes, err := list[pipelineJSON](p, ctx, path+"/pipelines", "")
	if err != nil {
		return out, fmt.Errorf("list GitLab pipelines: %w", err)
	}
	for _, r := range pipes {
		out.PipelineRuns = append(out.PipelineRuns, *ptrPipeline(r, repository))
	}
	return out, nil
}

func (p *Provider) TestRepository(ctx context.Context, repository string) (string, error) {
	path := "/api/v4/projects/" + url.PathEscape(repository)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+path, nil)
	if err != nil {
		return "", scm.ErrUnreachable
	}
	req.Header.Set("PRIVATE-TOKEN", p.token)
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", scm.ErrUnreachable
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", scm.ErrInvalidToken
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", scm.ErrProjectNotFound
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", scm.ErrUnreachable
	}
	var value struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&value); err != nil || value.Name == "" {
		return "", scm.ErrUnreachable
	}
	return value.Name, nil
}

func list[T any](p *Provider, ctx context.Context, path, extra string) ([]T, error) {
	var out []T
	for page := 1; ; page++ {
		q := "per_page=100&page=" + strconv.Itoa(page)
		if extra != "" {
			q += "&" + extra
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+path+"?"+q, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("PRIVATE-TOKEN", p.token)
		req.Header.Set("Accept", "application/json")
		resp, err := p.client.Do(req)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		closeErr := resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("GitLab API returned %s", resp.Status)
		}
		var values []T
		if err = json.Unmarshal(body, &values); err != nil {
			return nil, err
		}
		out = append(out, values...)
		next := resp.Header.Get("X-Next-Page")
		if next == "" {
			return out, nil
		}
	}
}
