package docs

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/swaggo/swag"
)

func TestGeneratedSwaggerContainsRequiredPaths(t *testing.T) {
	spec, err := swag.ReadDoc("swagger")
	require.NoError(t, err)
	require.Contains(t, spec, "/api/v1/webhooks/{provider}")
	require.Contains(t, spec, "/api/v1/auth/login")
	require.Contains(t, spec, "/api/v1/auth/session")
}

func TestGeneratedSwaggerDocumentsRepositorySyncErrorsAndAuth(t *testing.T) {
	spec, err := swag.ReadDoc("swagger")
	require.NoError(t, err)

	var document struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Schema struct {
					Ref string `json:"$ref"`
				} `json:"schema"`
			} `json:"responses"`
			Security []map[string][]string `json:"security"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(spec), &document))

	sync, ok := document.Paths["/api/v1/repositories/{repository_id}/sync"]["post"]
	require.True(t, ok)
	for _, status := range []string{"401", "403", "404", "500"} {
		require.Equal(t, "#/definitions/response.ErrorResponse", sync.Responses[status].Schema.Ref, "status %s", status)
	}
	require.Equal(t, []map[string][]string{{"BearerAuth": {}}}, sync.Security)
}
