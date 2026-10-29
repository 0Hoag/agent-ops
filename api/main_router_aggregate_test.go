package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/board"
	"github.com/0Hoag/agent-ops/api/pkg/config"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestProductionRouterAuthenticatedHistoricalAggregateIsOrganizationScoped(t *testing.T) {
	const secret = "aggregate-router-test-secret"
	const orgA = "d7000000-0000-4000-8000-000000000001"
	const orgB = "d7000000-0000-4000-8000-000000000002"
	const accountA = "d7000000-0000-4000-8000-000000000021"
	const accountB = "d7000000-0000-4000-8000-000000000022"
	const repoB = "d7000000-0000-4000-8000-000000000012"

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	router := httptest.NewServer(buildAggregateRouter(db, secret))
	t.Cleanup(router.Close)

	started := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	runColumns := []string{"id", "repository_id", "wave", "worker", "model", "issue_numbers", "pr_number", "input_tokens", "cached_input_tokens", "cache_write_input_tokens", "output_tokens", "reasoning_tokens", "cost_usd", "started_at", "ended_at", "created_at", "updated_at", "harness", "session_id", "billing_type"}
	priceColumns := []string{"id", "organization_id", "model", "effective_date", "input_per_million", "cached_input_per_million", "cache_write_per_million", "output_per_million", "reasoning_per_million", "created_at", "updated_at"}

	expectAggregate := func(org, account, repo, model string, input, output int64, inputRate, outputRate float64, issue int64, expectedCost string) {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "organization_memberships" WHERE organization_id = $1 AND account_id = $2`)).WithArgs(org, account).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		list := `SELECT .* FROM "agent_runs" JOIN repositories ON repositories.id = agent_runs.repository_id WHERE repositories.organization_id = \$1 AND repository_id = \$2 ORDER BY`
		if repo == "" {
			list = `SELECT .* FROM "agent_runs" JOIN repositories ON repositories.id = agent_runs.repository_id WHERE repositories.organization_id = \$1 ORDER BY`
		}
		rows := sqlmock.NewRows(runColumns)
		if issue != 0 {
			rows.AddRow("d7000000-0000-4000-8000-000000000071", repo, "W10", "worker", model, []byte(fmt.Sprintf("[%d]", issue)), nil, input, 0, 0, output, 0, nil, started, started, started, started, nil, nil, "api")
		}
		query := mock.ExpectQuery(list)
		if repo == "" {
			query.WithArgs(org)
		} else {
			query.WithArgs(org, repo)
		}
		query.WillReturnRows(rows)
		if issue != 0 {
			price := `SELECT .* FROM "agent_run_price_versions" WHERE organization_id = \$1 AND model = \$2 AND effective_date <= \$3 ORDER BY`
			mock.ExpectQuery(price).WithArgs(org, model, "2026-04-10", 1).WillReturnRows(sqlmock.NewRows(priceColumns).AddRow("d7000000-0000-4000-8000-000000000081", org, model, started.AddDate(0, -1, 0), inputRate, 0, 0, outputRate, 0, started, started))
		}
		assertJSON(t, router.URL+"/api/v1/agent-runs/aggregate"+func() string {
			if repo == "" {
				return ""
			}
			return "?repository_id=" + repo
		}(), tenantRouterToken(t, secret, account, org), http.StatusOK, fmt.Sprintf(`"issue_number":%d`, issue), `"cost_usd":`+expectedCost)
	}

	expectAggregate(orgA, accountA, "", "model-a", 1, 1, 1, 1, 1, "0.000002")
	expectAggregate(orgB, accountB, "", "model-b", 2, 2, 100, 100, 2, "0.0004")
	// The repository predicate remains present alongside the organization predicate.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "organization_memberships" WHERE organization_id = $1 AND account_id = $2`)).WithArgs(orgA, accountA).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT .* FROM "agent_runs" JOIN repositories ON repositories.id = agent_runs.repository_id WHERE repositories.organization_id = \$1 AND repository_id = \$2 ORDER BY`).WithArgs(orgA, repoB).WillReturnRows(sqlmock.NewRows(runColumns))
	assertJSON(t, router.URL+"/api/v1/agent-runs/aggregate?repository_id="+repoB, tenantRouterToken(t, secret, accountA, orgA), http.StatusOK, `"data":[]`)
	require.NoError(t, mock.ExpectationsWereMet())
}

func buildAggregateRouter(db *gorm.DB, secret string) http.Handler {
	router := gin.New()
	registerRoutesWithHub(router, database.AsPinger(db), db, config.Config{JWTSecret: secret, JWTExpiration: time.Hour}, board.NewHub())
	return router
}

func tenantRouterToken(t *testing.T, secret, account, org string) string {
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": account, "email": account + "@example.invalid", "organization_id": org, "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte(secret))
	require.NoError(t, err)
	return token
}
