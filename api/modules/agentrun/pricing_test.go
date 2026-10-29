package agentrun

import (
	"context"
	"errors"
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

func TestPriceAtSelectsBeforeOnAfterAndLeavesUnknownWithoutFallback(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	require.NoError(t, err)
	repo := NewRepository(db)
	query := regexp.QuoteMeta(`SELECT * FROM "agent_run_price_versions" WHERE organization_id = $1 AND model = $2 AND effective_date <= $3 ORDER BY effective_date DESC,"agent_run_price_versions"."id" LIMIT $4`)
	row := func(id, day string, input float64) *sqlmock.Rows {
		at, parseErr := time.Parse("2006-01-02", day)
		require.NoError(t, parseErr)
		return sqlmock.NewRows([]string{"id", "organization_id", "model", "effective_date", "input_per_million", "cached_input_per_million", "cache_write_per_million", "output_per_million", "reasoning_per_million", "created_at", "updated_at"}).AddRow(id, "org-a", "model-a", at, input, 0, 0, 0, 0, at, at)
	}
	mock.ExpectQuery(query).WithArgs("org-a", "model-a", "2026-04-09", 1).WillReturnRows(row("a1000000-0000-0000-0000-000000000001", "2026-04-01", 1))
	mock.ExpectQuery(query).WithArgs("org-a", "model-a", "2026-04-10", 1).WillReturnRows(row("a1000000-0000-0000-0000-000000000002", "2026-04-10", 2))
	mock.ExpectQuery(query).WithArgs("org-a", "model-a", "2026-04-11", 1).WillReturnRows(row("a1000000-0000-0000-0000-000000000003", "2026-04-10", 2))
	mock.ExpectQuery(query).WithArgs("org-a", "model-a", "2026-04-10", 1).WillReturnError(gorm.ErrRecordNotFound)
	for _, tc := range []struct {
		name, day string
		want      *float64
	}{
		{name: "before selects prior version", day: "2026-04-09", want: func() *float64 { x := 1.0; return &x }()},
		{name: "on selects effective version", day: "2026-04-10", want: func() *float64 { x := 2.0; return &x }()},
		{name: "after selects latest applicable version", day: "2026-04-11", want: func() *float64 { x := 2.0; return &x }()},
		{name: "unknown stays unknown", day: "2026-04-10", want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.priceAt(context.Background(), "org-a", "model-a", mustDate(t, tc.day))
			require.NoError(t, err)
			if tc.want == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, *tc.want, got.InputPerMillion)
		})
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func mustDate(t *testing.T, value string) time.Time {
	t.Helper()
	got, err := time.Parse("2006-01-02", value)
	require.NoError(t, err)
	return got
}

func TestAggregateHistoricalSeparatesSubscriptionBilledZeroFromEquivalentAPI(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	require.NoError(t, err)
	repo := NewRepository(db)
	started := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	harness := "codex"
	sessionAPI, sessionSub := "api-session", "sub-session"
	rows := sqlmock.NewRows([]string{"id", "repository_id", "wave", "worker", "model", "issue_numbers", "pr_number", "input_tokens", "cached_input_tokens", "cache_write_tokens", "output_tokens", "reasoning_tokens", "cost_usd", "started_at", "ended_at", "created_at", "updated_at", "harness", "session_id", "billing_type"}).
		AddRow("a1000000-0000-0000-0000-000000000001", "b1000000-0000-0000-0000-000000000001", "W10", "worker", "model-a", []byte("[63]"), nil, 100, 0, 0, 100, 0, nil, started, started, started, started, harness, sessionAPI, "api").
		AddRow("a1000000-0000-0000-0000-000000000002", "b1000000-0000-0000-0000-000000000001", "W10", "worker", "model-a", []byte("[63]"), nil, 100, 0, 0, 100, 0, nil, started, started, started, started, harness, sessionSub, "subscription")
	mock.ExpectQuery(`SELECT .* FROM "agent_runs"`).WithArgs("org-a").WillReturnRows(rows)
	priceQuery := regexp.QuoteMeta(`SELECT * FROM "agent_run_price_versions" WHERE organization_id = $1 AND model = $2 AND effective_date <= $3 ORDER BY effective_date DESC,"agent_run_price_versions"."id" LIMIT $4`)
	priceRow := sqlmock.NewRows([]string{"id", "organization_id", "model", "effective_date", "input_per_million", "cached_input_per_million", "cache_write_per_million", "output_per_million", "reasoning_per_million", "created_at", "updated_at"}).AddRow("a1000000-0000-0000-0000-000000000010", "org-a", "model-a", mustDate(t, "2026-04-01"), 1, 0, 0, 2, 0, started, started)
	mock.ExpectQuery(priceQuery).WithArgs("org-a", "model-a", "2026-04-10", 1).WillReturnRows(priceRow)
	mock.ExpectQuery(priceQuery).WithArgs("org-a", "model-a", "2026-04-10", 1).WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "model", "effective_date", "input_per_million", "cached_input_per_million", "cache_write_per_million", "output_per_million", "reasoning_per_million", "created_at", "updated_at"}).AddRow("a1000000-0000-0000-0000-000000000010", "org-a", "model-a", mustDate(t, "2026-04-01"), 1, 0, 0, 2, 0, started, started))
	got, err := repo.AggregateHistorical(WithOrganization(context.Background(), "org-a"), Filter{}, nil)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].EquivalentCostUSD)
	require.InDelta(t, 0.0006, *got[0].EquivalentCostUSD, 1e-12)
	require.NotNil(t, got[0].BilledCostUSD)
	require.InDelta(t, 0.0003, *got[0].BilledCostUSD, 1e-12)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAggregateHistoricalUnknownPriceRemainsUnknown(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	require.NoError(t, err)
	repo := NewRepository(db)
	started := mustDate(t, "2026-04-10")
	rows := sqlmock.NewRows([]string{"id", "repository_id", "wave", "worker", "model", "issue_numbers", "pr_number", "input_tokens", "cached_input_tokens", "cache_write_tokens", "output_tokens", "reasoning_tokens", "cost_usd", "started_at", "ended_at", "created_at", "updated_at", "harness", "session_id", "billing_type"}).AddRow("a1000000-0000-0000-0000-000000000003", "b1000000-0000-0000-0000-000000000001", "W10", "worker", "unknown-model", []byte("[64]"), nil, 100, 0, 0, 0, 0, nil, started, started, started, started, "codex", "unknown-session", "api")
	mock.ExpectQuery(`SELECT .* FROM "agent_runs"`).WithArgs("org-a").WillReturnRows(rows)
	priceQuery := regexp.QuoteMeta(`SELECT * FROM "agent_run_price_versions" WHERE organization_id = $1 AND model = $2 AND effective_date <= $3 ORDER BY effective_date DESC,"agent_run_price_versions"."id" LIMIT $4`)
	mock.ExpectQuery(priceQuery).WithArgs("org-a", "unknown-model", "2026-04-10", 1).WillReturnError(gorm.ErrRecordNotFound)
	got, err := repo.AggregateHistorical(WithOrganization(context.Background(), "org-a"), Filter{}, nil)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Nil(t, got[0].CostUSD)
	require.Nil(t, got[0].EquivalentCostUSD)
	require.Nil(t, got[0].BilledCostUSD)
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
	prices       []PriceVersion
	updatedPrice PriceVersion
	created      bool
	updated      bool
	org          string
	id           string
}

type duplicatePricingRepo struct{ calls int }

func (p *duplicatePricingRepo) ListPrices(context.Context, string) ([]PriceVersion, error) {
	return nil, nil
}
func (p *duplicatePricingRepo) CreatePrice(context.Context, *PriceVersion) error {
	p.calls++
	if p.calls > 1 {
		return errors.New("duplicate key value violates unique constraint agent_run_price_versions_org_model_date")
	}
	return nil
}
func (p *duplicatePricingRepo) UpdatePrice(context.Context, string, string, *PriceVersion) error {
	return nil
}

func (p *pricingRepo) ListPrices(_ context.Context, org string) ([]PriceVersion, error) {
	p.org = org
	return p.prices, nil
}
func (p *pricingRepo) CreatePrice(_ context.Context, price *PriceVersion) error {
	p.created = true
	p.prices = append(p.prices, *price)
	return nil
}
func (p *pricingRepo) UpdatePrice(_ context.Context, org, id string, price *PriceVersion) error {
	p.org, p.id = org, id
	p.updatedPrice = *price
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

func TestPriceControllerListAndUpdateAreTenantScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &pricingRepo{prices: []PriceVersion{{Model: "model-a"}}}
	membership := pricingMemberships{role: "owner"}
	r := gin.New()
	c := NewPriceController(repo, membership)
	r.GET("/", func(ctx *gin.Context) {
		ctx.Set("organization_id", "org-a")
		ctx.Set("account_id", "acct-a")
		c.List(ctx)
	})
	r.PATCH("/:price_id", func(ctx *gin.Context) {
		ctx.Set("organization_id", "org-a")
		ctx.Set("account_id", "acct-a")
		c.Update(ctx)
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	req := httptest.NewRequest(http.MethodPatch, "/price-1", strings.NewReader(`{"model":"model-b","effective_date":"2026-04-10T00:00:00Z","input_per_million":1,"cached_input_per_million":1,"cache_write_per_million":1,"output_per_million":1,"reasoning_per_million":1}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(httptest.NewRecorder(), req)
	require.Equal(t, "org-a", repo.org)
	require.Equal(t, "price-1", repo.id)
}

func TestPriceControllerOwnerAndAdminCanWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"model-a","effective_date":"2026-04-10T00:00:00Z","input_per_million":1,"cached_input_per_million":1,"cache_write_per_million":1,"output_per_million":1,"reasoning_per_million":1}`
	for _, role := range []string{"owner", "admin"} {
		t.Run(role, func(t *testing.T) {
			repo := &pricingRepo{}
			r := gin.New()
			r.POST("/", func(ctx *gin.Context) {
				ctx.Set("organization_id", "org-a")
				ctx.Set("account_id", "acct-a")
				NewPriceController(repo, pricingMemberships{role: role}).Create(ctx)
			})
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			r.ServeHTTP(res, req)
			require.Equal(t, http.StatusCreated, res.Code)
			require.True(t, repo.created)
			require.Len(t, repo.prices, 1)
			require.Equal(t, "org-a", string(repo.prices[0].OrganizationID))
		})
	}
}

func TestPriceControllerDateOnlyCreateAndPatchPersistUTCDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dateOnlyBody := `{"model":"model-a","effective_date":"2027-01-01","input_per_million":1,"cached_input_per_million":1,"cache_write_per_million":1,"output_per_million":1,"reasoning_per_million":1}`
	repo := &pricingRepo{}
	r := gin.New()
	c := NewPriceController(repo, pricingMemberships{role: "admin"})
	r.POST("/", func(ctx *gin.Context) {
		ctx.Set("organization_id", "org-a")
		ctx.Set("account_id", "acct-a")
		c.Create(ctx)
	})
	r.PATCH("/:price_id", func(ctx *gin.Context) {
		ctx.Set("organization_id", "org-a")
		ctx.Set("account_id", "acct-a")
		c.Update(ctx)
	})
	post := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(dateOnlyBody))
	post.Header.Set("Content-Type", "application/json")
	postResponse := httptest.NewRecorder()
	r.ServeHTTP(postResponse, post)
	require.Equal(t, http.StatusCreated, postResponse.Code)
	require.Len(t, repo.prices, 1)
	require.Equal(t, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), repo.prices[0].EffectiveDate)

	patch := httptest.NewRequest(http.MethodPatch, "/price-1", strings.NewReader(dateOnlyBody))
	patch.Header.Set("Content-Type", "application/json")
	patchResponse := httptest.NewRecorder()
	r.ServeHTTP(patchResponse, patch)
	require.Equal(t, http.StatusOK, patchResponse.Code)
	require.Equal(t, "org-a", repo.org)
	require.Equal(t, "price-1", repo.id)
	require.Equal(t, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), repo.updatedPrice.EffectiveDate)
}

func TestPriceControllerRejectsMalformedOrImpossibleDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &pricingRepo{}
	r := gin.New()
	r.POST("/", func(ctx *gin.Context) {
		ctx.Set("organization_id", "org-a")
		ctx.Set("account_id", "acct-a")
		NewPriceController(repo, pricingMemberships{role: "admin"}).Create(ctx)
	})
	for _, date := range []string{"2027-02-29", "2027-1-01", "2027-01-01T01:00:00Z"} {
		t.Run(date, func(t *testing.T) {
			body := `{"model":"model-a","effective_date":"` + date + `","input_per_million":1}`
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			r.ServeHTTP(res, req)
			require.Equal(t, http.StatusBadRequest, res.Code)
		})
	}
	require.False(t, repo.created)
}

func TestPriceControllerDuplicateVersionReturnsConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"model-a","effective_date":"2026-04-10T00:00:00Z","input_per_million":1,"cached_input_per_million":1,"cache_write_per_million":1,"output_per_million":1,"reasoning_per_million":1}`
	repo := &duplicatePricingRepo{}
	r := gin.New()
	r.POST("/", func(ctx *gin.Context) {
		ctx.Set("organization_id", "org-a")
		ctx.Set("account_id", "acct-a")
		NewPriceController(repo, pricingMemberships{role: "admin"}).Create(ctx)
	})
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		if i == 0 {
			require.Equal(t, http.StatusCreated, res.Code)
		} else {
			require.Equal(t, http.StatusConflict, res.Code)
		}
	}
	require.Equal(t, 2, repo.calls)
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
