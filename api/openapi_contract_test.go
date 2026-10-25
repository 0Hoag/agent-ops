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
		Paths       map[string]map[string]json.RawMessage `json:"paths"`
		Definitions map[string]json.RawMessage            `json:"definitions"`
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
		op, ok := document.Paths[operationPath][strings.ToLower(method)]
		require.Truef(t, ok, "missing OpenAPI operation for registered route %s %s (generated path %s)", method, path, operationPath)
		if method == http.MethodGet && path == "/api/v1/ws" {
			continue // 101 is an upgrade handshake, not a JSON response.
		}
		var operation map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(op, &operation))
		for _, field := range []string{"parameters", "responses"} {
			raw, exists := operation[field]
			if field == "parameters" && !exists {
				continue
			}
			require.Truef(t, exists, "missing %s schema coverage for registered route %s %s", field, method, path)
			var encoded any
			require.NoError(t, json.Unmarshal(raw, &encoded))
			refs := make([]string, 0)
			collectSchemaRefs(encoded, &refs)
			if field == "responses" {
				require.NotEmptyf(t, refs, "missing response schema for registered route %s %s", method, path)
			}
			for _, ref := range refs {
				const prefix = "#/definitions/"
				if strings.HasPrefix(ref, prefix) {
					name := strings.TrimPrefix(ref, prefix)
					require.Containsf(t, document.Definitions, name, "unresolved schema reference %s for registered route %s %s", ref, method, path)
				}
			}
		}
	}
}

func collectSchemaRefs(value any, refs *[]string) {
	switch v := value.(type) {
	case map[string]any:
		if ref, ok := v["$ref"].(string); ok {
			*refs = append(*refs, ref)
		}
		for _, child := range v {
			collectSchemaRefs(child, refs)
		}
	case []any:
		for _, child := range v {
			collectSchemaRefs(child, refs)
		}
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
