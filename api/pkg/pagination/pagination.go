// Package pagination provides utilities for parsing pagination parameters.
package pagination

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	DefaultPage    = 1
	DefaultPerPage = 50
	MaxPerPage     = 100
)

type Params struct{ Page, PerPage, Offset int }

func Parse(c *gin.Context) Params { return ParseWithMax(c, MaxPerPage) }

func ParseWithMax(c *gin.Context, maxPerPage int) Params {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	if page < 1 {
		page = DefaultPage
	}
	if perPage < 1 || perPage > maxPerPage {
		perPage = DefaultPerPage
	}
	return Params{Page: page, PerPage: perPage, Offset: (page - 1) * perPage}
}

func (p Params) ToResponse(total int64) PaginatorResponse {
	return PaginatorResponse{Page: p.Page, PerPage: p.PerPage, Total: total}
}

type PaginatorResponse struct {
	Page    int   `json:"page"`
	PerPage int   `json:"per_page"`
	Total   int64 `json:"total"`
}
