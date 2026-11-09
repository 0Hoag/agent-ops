package insight

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestOverviewPeriodCostAndCacheArithmetic(t *testing.T) {
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	run := AnalyticsRun{
		Model:             "model-a",
		InputTokens:       1_000_000,
		CachedInputTokens: 500_000,
		CacheWriteTokens:  250_000,
		OutputTokens:      250_000,
		ReasoningTokens:   100_000,
	}
	price := priceRow{Model: "model-a", EffectiveDate: at.Add(-time.Hour), InputPerMillion: 2, CachedInputPerMillion: 0.5, CacheWritePerMillion: 1, OutputPerMillion: 4, ReasoningPerMillion: 3}

	got := cost(run, price)
	require.Equal(t, 3.8, got)
	require.Equal(t, int64(1_750_000), run.InputTokens+run.CachedInputTokens+run.CacheWriteTokens)
	require.Equal(t, price, func() priceRow {
		p, ok := priceAt([]priceRow{price}, "model-a", at)
		require.True(t, ok)
		return p
	}())
	_, ok := priceAt([]priceRow{price}, "missing-model", at)
	require.False(t, ok)
}

func TestOverviewRejectsInvalidRangeWithoutDatabaseAccess(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = sqlDB.Close() }()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)

	from := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	_, err = NewRepository(db).Overview(context.Background(), OverviewFilter{From: from, To: from.Add(-24 * time.Hour)})
	require.EqualError(t, err, "invalid overview date range")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOverviewControllerParsesInclusiveDateQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/overview", (&Controller{repository: nil}).Overview)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/overview?from=bad&to=2026-11-09", nil)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Contains(t, res.Body.String(), "from must be YYYY-MM-DD")
}

func TestCompatibleTimelinePreservesCumulativeTotalsAcrossThreeDays(t *testing.T) {
	run := AnalyticsRun{InputTokens: 60, CachedInputTokens: 30, CacheWriteTokens: 15, OutputTokens: 9, ReasoningTokens: 6}
	base := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	steps := []usageStep{
		{StartedAt: timePtr(base), InputTokens: 10, CachedInputTokens: 5, CacheWriteTokens: 5, OutputTokens: 2, ReasoningTokens: 2},
		{StartedAt: timePtr(base.Add(24 * time.Hour)), InputTokens: 20, CachedInputTokens: 10, CacheWriteTokens: 5, OutputTokens: 3, ReasoningTokens: 2},
		{StartedAt: timePtr(base.Add(48 * time.Hour)), InputTokens: 30, CachedInputTokens: 15, CacheWriteTokens: 5, OutputTokens: 4, ReasoningTokens: 2},
	}
	require.True(t, compatibleTimeline(run, steps))
	var got AnalyticsRun
	for _, step := range steps {
		part := stepRun(run, step)
		got.InputTokens += part.InputTokens
		got.CachedInputTokens += part.CachedInputTokens
		got.CacheWriteTokens += part.CacheWriteTokens
		got.OutputTokens += part.OutputTokens
		got.ReasoningTokens += part.ReasoningTokens
	}
	require.Equal(t, run.InputTokens, got.InputTokens)
	require.Equal(t, run.CachedInputTokens, got.CachedInputTokens)
	require.Equal(t, run.CacheWriteTokens, got.CacheWriteTokens)
	require.Equal(t, run.OutputTokens, got.OutputTokens)
	require.Equal(t, run.ReasoningTokens, got.ReasoningTokens)
}

func timePtr(t time.Time) *time.Time { return &t }
