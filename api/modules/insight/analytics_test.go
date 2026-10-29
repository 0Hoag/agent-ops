package insight

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAnalyticsCostRecomputesTokenCategories(t *testing.T) {
	run := AnalyticsRun{InputTokens: 100, CachedInputTokens: 200, CacheWriteTokens: 50, OutputTokens: 300, ReasoningTokens: 25}
	p := priceRow{InputPerMillion: 1, CachedInputPerMillion: 2, CacheWritePerMillion: 3, OutputPerMillion: 4, ReasoningPerMillion: 5}
	// Independent fixture recomputation: each category is priced separately before summing.
	expected := (100*1 + 200*2 + 50*3 + 300*4 + 25*5) / 1_000_000.0
	require.Equal(t, expected, cost(run, p))
}

func TestAnalyticsPriceSelectionUsesHistoricalEffectiveDate(t *testing.T) {
	at := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	prices := []priceRow{{Model: "m", EffectiveDate: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), InputPerMillion: 1}, {Model: "m", EffectiveDate: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), InputPerMillion: 2}}
	got, ok := priceAt(prices, "m", at)
	require.True(t, ok)
	require.Equal(t, 2.0, got.InputPerMillion)
}

func TestAnalyticsWeekBucketsStartMondayUTC(t *testing.T) {
	got := monday(time.Date(2026, 4, 12, 23, 0, 0, 0, time.UTC))
	require.Equal(t, time.Date(2026, 4, 6, 23, 0, 0, 0, time.UTC), got)
}
