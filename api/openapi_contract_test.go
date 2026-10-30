package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

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
	registerRoutes(router, nil, nil, config.Config{JWTSecret: "openapi-test-key", JWTExpiration: time.Hour})
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
		var operation map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(op, &operation))
		if method == http.MethodPost && path == "/api/v1/webhooks/:provider" {
			assertWebhookObjectBody(t, operation, document.Definitions, method, path)
		}
		for _, field := range []string{"parameters", "responses"} {
			raw, exists := operation[field]
			if field == "parameters" && !exists {
				continue
			}
			require.Truef(t, exists, "missing %s schema coverage for registered route %s %s", field, method, path)
			if field == "responses" {
				var responses map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(raw, &responses))
				require.NotEmptyf(t, responses, "empty responses map for registered route %s %s", method, path)
				for code, response := range responses {
					if method == http.MethodGet && (path == "/api/v1/ws" || path == "/api/v1/board/ws") && code == "101" {
						continue
					}
					var responseObject map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(response, &responseObject))
					schema, hasSchema := responseObject["schema"]
					require.Truef(t, hasSchema, "missing response schema for registered route %s %s response %s", method, path, code)
					var schemaValue any
					require.NoError(t, json.Unmarshal(schema, &schemaValue))
					responseRefs := make([]string, 0)
					collectSchemaRefs(schemaValue, &responseRefs)
					if method == http.MethodGet && path == "/metrics" {
						continue // Prometheus exposition is a plain-text scalar, not a JSON definition.
					}
					require.NotEmptyf(t, responseRefs, "missing response schema reference for registered route %s %s response %s", method, path, code)
					for _, ref := range responseRefs {
						assertDefinitionResolves(t, document.Definitions, ref, method, path, make(map[string]bool))
					}
				}
			}
			var encoded any
			require.NoError(t, json.Unmarshal(raw, &encoded))
			refs := make([]string, 0)
			collectSchemaRefs(encoded, &refs)
			for _, ref := range refs {
				assertDefinitionResolves(t, document.Definitions, ref, method, path, make(map[string]bool))
			}
		}
	}
}

func assertWebhookObjectBody(t *testing.T, operation map[string]json.RawMessage, definitions map[string]json.RawMessage, method, path string) {
	t.Helper()
	var parameters []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(operation["parameters"], &parameters))
	for _, parameter := range parameters {
		var name, in string
		require.NoError(t, json.Unmarshal(parameter["name"], &name))
		require.NoError(t, json.Unmarshal(parameter["in"], &in))
		if name != "body" || in != "body" {
			continue
		}
		var schema map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(parameter["schema"], &schema))
		var typ string
		if ref, ok := schema["$ref"]; ok {
			var reference string
			require.NoError(t, json.Unmarshal(ref, &reference))
			const prefix = "#/definitions/"
			require.Truef(t, strings.HasPrefix(reference, prefix), "unsupported webhook body schema reference %s for registered route %s %s", reference, method, path)
			definition, exists := definitions[strings.TrimPrefix(reference, prefix)]
			require.Truef(t, exists, "unresolved webhook body schema reference %s for registered route %s %s", reference, method, path)
			require.NoError(t, json.Unmarshal(definition, &schema))
		}
		require.NoError(t, json.Unmarshal(schema["type"], &typ))
		require.Equalf(t, "object", typ, "webhook body schema must be a JSON object for registered route %s %s", method, path)
		return
	}
	require.Failf(t, "missing webhook body schema", "registered route %s %s", method, path)
}

func assertDefinitionResolves(t *testing.T, definitions map[string]json.RawMessage, ref, method, path string, visited map[string]bool) {
	t.Helper()
	const prefix = "#/definitions/"
	require.Truef(t, strings.HasPrefix(ref, prefix), "unsupported schema reference %s for registered route %s %s", ref, method, path)
	name := strings.TrimPrefix(ref, prefix)
	require.Containsf(t, definitions, name, "unresolved schema reference %s for registered route %s %s", ref, method, path)
	if visited[name] {
		return
	}
	visited[name] = true
	var definition any
	require.NoError(t, json.Unmarshal(definitions[name], &definition))
	refs := make([]string, 0)
	collectSchemaRefs(definition, &refs)
	for _, nested := range refs {
		assertDefinitionResolves(t, definitions, nested, method, path, visited)
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
