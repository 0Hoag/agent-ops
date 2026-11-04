package worker

import "github.com/0Hoag/agent-ops/api/pkg/pagination"

type AuditHistoryResponse struct {
	Data []ActionAudit                `json:"data"`
	Meta pagination.PaginatorResponse `json:"meta"`
}
