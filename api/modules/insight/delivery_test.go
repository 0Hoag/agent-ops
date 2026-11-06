package insight

import (
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/stretchr/testify/require"
)

func TestDeliveryCostUsesHistoricalPriceAndUnknownIsUnavailable(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	run := deliveryRun{Model: "m", StartedAt: at, InputTokens: 100, OutputTokens: 50}
	prices := []priceRow{{Model: "m", EffectiveDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), InputPerMillion: 2, OutputPerMillion: 4}}
	got, ok := deliveryCost(run, prices)
	require.True(t, ok)
	require.Equal(t, (100*2+50*4)/1_000_000.0, got)
	_, ok = deliveryCost(deliveryRun{Model: "missing", StartedAt: at}, prices)
	require.False(t, ok)
}

func TestDeliveryDiffEvidenceRequiresBothProviderCounts(t *testing.T) {
	p := int64(9)
	withDiff := decode(store.WebhookEvent{Provider: "github", EventType: "pull_request", DeliveryID: "a", Payload: store.JSONB(`{"pull_request":{"number":9,"additions":3,"deletions":2}}`)})
	withoutDiff := decode(store.WebhookEvent{Provider: "github", EventType: "pull_request", DeliveryID: "b", Payload: store.JSONB(`{"pull_request":{"number":9,"additions":3}}`)})
	require.Equal(t, &p, withDiff.pr)
	require.Equal(t, int64(3), *withDiff.additions)
	require.Equal(t, int64(2), *withDiff.deletions)
	require.Nil(t, withoutDiff.deletions)
}

func TestDeliveryHarnessMetricNormalizesByObservedChangedLines(t *testing.T) {
	var metrics []HarnessMetric
	run := deliveryRun{Harness: stringPtr("codex")}
	addHarness(&metrics, "codex", 0.50, run, 10, true)
	addHarness(&metrics, "codex", 0.25, run, 5, true)
	got := *metrics[0].EquivalentCostUSD / float64(metrics[0].ChangedLines)
	require.Equal(t, 2, metrics[0].Eligible)
	require.Equal(t, int64(15), metrics[0].ChangedLines)
	require.Equal(t, 0.75/15.0, got)
}

func stringPtr(v string) *string { return &v }
