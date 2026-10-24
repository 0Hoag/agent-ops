package insight

import (
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/stretchr/testify/require"
)

func ev(id, status string, at time.Time, issue int64) event {
	return event{WebhookEvent: store.WebhookEvent{DeliveryID: id, EventType: "issues", ReceivedAt: at}, status: status, issue: &issue}
}

func TestSendBackDeduplicatedTransition(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := sendBack([]event{ev("a", "in_progress", t0, 7), ev("b", "review", t0.Add(time.Hour), 7), ev("b", "review", t0.Add(time.Hour), 7), ev("c", "in_progress", t0.Add(2*time.Hour), 7), ev("d", "review", t0.Add(3*time.Hour), 7)})
	require.Equal(t, 2, got.Denominator)
	require.Equal(t, 2, got.Numerator)
	require.Equal(t, 1.0, got.Rate)
}

func TestFirstCIUsesFirstConclusionPerSHA(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	got := firstCI([]event{{WebhookEvent: store.WebhookEvent{DeliveryID: "1", ReceivedAt: base}, sha: "a", conclusion: "success"}, {WebhookEvent: store.WebhookEvent{DeliveryID: "2", ReceivedAt: base.Add(time.Minute)}, sha: "a", conclusion: "failure"}, {WebhookEvent: store.WebhookEvent{DeliveryID: "3", ReceivedAt: base}, sha: "b", conclusion: "failure"}, {WebhookEvent: store.WebhookEvent{DeliveryID: "4", ReceivedAt: base}, sha: "c"}})
	require.Equal(t, 2, got.Eligible)
	require.Equal(t, 1, got.Successful)
	require.Equal(t, .5, got.Rate)
}

func TestCycleTimeExcludesIncompleteAndIncludesBlocked(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	got := cycle([]event{ev("a", "in_progress", base, 1), ev("b", "blocked", base.Add(time.Hour), 1), ev("c", "done", base.Add(3*time.Hour), 1), ev("d", "in_progress", base, 2)})
	require.Equal(t, 1, got.Completed)
	require.Equal(t, float64(3*time.Hour/time.Second), got.MedianSeconds)
}

func TestSuggestionsRequireRecurringFailures(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	got := suggestions([]event{{WebhookEvent: store.WebhookEvent{DeliveryID: "1", ReceivedAt: base}, sha: "same", conclusion: "failure"}, {WebhookEvent: store.WebhookEvent{DeliveryID: "2", ReceivedAt: base.Add(time.Minute)}, sha: "same", conclusion: "failure"}, {WebhookEvent: store.WebhookEvent{DeliveryID: "3", ReceivedAt: base}, sha: "one", conclusion: "failure"}})
	require.Len(t, got, 1)
	require.Equal(t, 2, got[0].Occurrences)
	require.Len(t, got[0].Evidence, 2)
}
