package board

import (
	"encoding/json"

	"github.com/0Hoag/agent-ops/api/modules/store"
)

type RepositoryDTO struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	FullName string `json:"full_name"`
}

type PipelineRunDTO struct {
	ExternalID string `json:"external_id"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	SHA        string `json:"sha"`
}
type PullRequestDTO struct {
	Number       int64            `json:"number"`
	Title        string           `json:"title"`
	State        string           `json:"state"`
	HeadSHA      string           `json:"head_sha"`
	Branch       string           `json:"branch"`
	PipelineRuns []PipelineRunDTO `json:"pipeline_runs"`
}
type IssueDTO struct {
	ID           string           `json:"id"`
	RepositoryID string           `json:"repository_id"`
	Number       int64            `json:"number"`
	Title        string           `json:"title"`
	State        string           `json:"state"`
	Status       string           `json:"status"`
	Labels       json.RawMessage  `json:"labels"`
	Assignee     string           `json:"assignee"`
	PullRequests []PullRequestDTO `json:"pull_requests"`
}
type ListMeta struct {
	Page    int   `json:"page"`
	PerPage int   `json:"per_page"`
	Total   int64 `json:"total"`
}
type ListResponse struct {
	Data any      `json:"data"`
	Meta ListMeta `json:"meta"`
}

func repositoryDTO(item store.Repository) RepositoryDTO {
	return RepositoryDTO{ID: string(item.ID), Provider: item.Provider, FullName: item.FullName}
}
