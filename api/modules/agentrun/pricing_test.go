package agentrun

import (
	"testing"

	"github.com/stretchr/testify/require"
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
