package agentrun

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPriceCost(t *testing.T) {
	run := AgentRun{InputTokens: 100, CachedInputTokens: 200, CacheWriteTokens: 50, OutputTokens: 300, ReasoningTokens: 25}
	price := Price{InputPerMillion: 1, CachedInputPerMillion: .1, CacheWritePerMillion: 2, OutputPerMillion: 3, ReasoningPerMillion: 4}
	tests := []struct {
		name string
		run  AgentRun
		want float64
	}{
		{"cache ratios and arithmetic", run, .0001 + .00002 + .0001 + .0009 + .0001},
		{"zero usage", AgentRun{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { require.InDelta(t, tt.want, priceCost(tt.run, price), 1e-12) })
	}
}

func TestPriceAtUsesTenantModelAndInclusiveEffectiveDate(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	require.NoError(t, err)
	repo := NewRepository(db)
	id := "a1000000-0000-0000-0000-000000000001"
	day := time.Date(2026, 4, 10, 12, 0, 0, 0, time.FixedZone("ICT", 7*60*60))
	price := sqlmock.NewRows([]string{"id", "organization_id", "model", "effective_date", "input_per_million", "cached_input_per_million", "cache_write_per_million", "output_per_million", "reasoning_per_million", "created_at", "updated_at"}).AddRow(id, "org-a", "model-a", time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), 2, 1, 3, 4, 5, time.Now(), time.Now())
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_run_price_versions" WHERE organization_id = $1 AND model = $2 AND effective_date <= $3 ORDER BY effective_date DESC,"agent_run_price_versions"."id" LIMIT $4`)).WithArgs("org-a", "model-a", "2026-04-10", 1).WillReturnRows(price)
	got, err := repo.priceAt(context.Background(), "org-a", "model-a", day)
	require.NoError(t, err)
	require.Equal(t, &Price{InputPerMillion: 2, CachedInputPerMillion: 1, CacheWritePerMillion: 3, OutputPerMillion: 4, ReasoningPerMillion: 5}, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

type pricingMemberships struct {
	role string
	err  error
}

func (m pricingMemberships) Membership(context.Context, string, string) (*organization.Membership, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &organization.Membership{Role: m.role}, nil
}

type pricingRepo struct {
	created bool
	updated bool
}

func (p *pricingRepo) ListPrices(context.Context, string) ([]PriceVersion, error) { return nil, nil }
func (p *pricingRepo) CreatePrice(context.Context, *PriceVersion) error           { p.created = true; return nil }
func (p *pricingRepo) UpdatePrice(context.Context, string, string, *PriceVersion) error {
	p.updated = true
	return nil
}

func TestPriceControllerEnforcesMembershipAndRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"model-a","effective_date":"2026-04-10T00:00:00Z","input_per_million":1,"cached_input_per_million":1,"cache_write_per_million":1,"output_per_million":1,"reasoning_per_million":1}`
	for _, tc := range []struct {
		name, role    string
		membershipErr error
		want          int
	}{
		{"member rejected", "member", nil, http.StatusForbidden},
		{"viewer rejected", "viewer", nil, http.StatusForbidden},
		{"non member rejected", "", organization.ErrNotMember, http.StatusForbidden},
		{"admin accepted", "admin", nil, http.StatusCreated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &pricingRepo{}
			r := gin.New()
			r.POST("/", func(c *gin.Context) {
				c.Set("organization_id", "org-a")
				c.Set("account_id", "acct-a")
				NewPriceController(repo, pricingMemberships{role: tc.role, err: tc.membershipErr}).Create(c)
			})
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			r.ServeHTTP(res, req)
			require.Equal(t, tc.want, res.Code)
			require.Equal(t, tc.want == http.StatusCreated, repo.created)
		})
	}
}

func TestPriceCostHistoricalFixtures(t *testing.T) {
	p := Price{InputPerMillion: 2, CachedInputPerMillion: 1, CacheWritePerMillion: 4, OutputPerMillion: 3, ReasoningPerMillion: 5}
	for _, tc := range []struct {
		name string
		run  AgentRun
		want float64
	}{
		{name: "cache heavy", run: AgentRun{InputTokens: 10, CachedInputTokens: 900, CacheWriteTokens: 20, OutputTokens: 10}, want: .00002 + .0009 + .00008 + .00003},
		{name: "reasoning heavy", run: AgentRun{OutputTokens: 100, ReasoningTokens: 900}, want: .0003 + .0045},
		{name: "zero tokens", run: AgentRun{}, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) { require.InDelta(t, tc.want, priceCost(tc.run, p), 1e-12) })
	}
}
