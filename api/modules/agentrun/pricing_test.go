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
