package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	_ "github.com/0Hoag/agent-ops/api/docs"
	"github.com/0Hoag/agent-ops/api/pkg/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/swaggo/swag"
)

// TestOpenAPI covers the contract from the router's registered product routes,
// so adding a route without documenting it cannot silently pass by updating a list.
func TestOpenAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerRoutes(router, nil, nil, config.Config{})
	if path := os.Getenv("OPENAPI_TEST_ROUTE"); path != "" {
		router.GET(path, func(c *gin.Context) {})
	}

	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	doc, err := swag.ReadDoc()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(doc), &document))

	for _, route := range router.Routes() {
		if route.Path == "/swagger/*any" {
			continue // Swagger UI/assets are documentation infrastructure, not product API.
		}
		path := route.Path
		method := route.Method
		operationPath := ginToOpenAPIPath(path)
		_, ok := document.Paths[operationPath][strings.ToLower(method)]
		require.Truef(t, ok, "missing OpenAPI operation for registered route %s %s (generated path %s)", method, path, operationPath)
	}

	for _, check := range []struct {
		path, method, parameter, schema string
	}{
		{"/api/v1/webhooks/:provider", http.MethodPost, "provider", "response.SuccessResponse"},
		{"/api/v1/board", http.MethodGet, "repository_id", "board.ListResponse"},
		{"/api/v1/agent-runs", http.MethodPost, "X-Agent-Run-Token", "agentrun.CreateInput"},
	} {
		op := document.Paths[ginToOpenAPIPath(check.path)][strings.ToLower(check.method)]
		require.Contains(t, string(op), `"name": "`+check.parameter+`"`, "important route parameter missing from generated operation")
		require.Contains(t, string(op), check.schema, "important request/response schema missing from generated operation")
	}
}

func ginToOpenAPIPath(path string) string {
	result := ""
	for i := 0; i < len(path); i++ {
		if path[i] == ':' {
			j := i + 1
			for j < len(path) && path[j] != '/' {
				j++
			}
			result += "{" + path[i+1:j] + "}"
			i = j - 1
			continue
		}
		result += string(path[i])
	}
	return result
}
