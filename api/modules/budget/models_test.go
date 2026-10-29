package budget

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEvaluateThresholds(t *testing.T) {
	tok := int64(100)
	usd := 10.0
	b := Budget{TokenLimit: &tok, USDLimit: &usd}
	for _, tc := range []struct {
		name string
		u    Usage
		want Threshold
	}{{"below", Usage{79, 7.9}, Threshold{}}, {"at80", Usage{80, 8}, Threshold{true, false, true, false}}, {"at100", Usage{100, 10}, Threshold{true, true, true, true}}, {"above", Usage{101, 11}, Threshold{true, true, true, true}}} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Evaluate(b, tc.u); got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestValidBudgetRequest(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	token := int64(10)
	usd := 1.0
	for name, tc := range map[string]struct {
		req  Request
		want bool
	}{
		"token limit":     {Request{TokenLimit: &token, StartsAt: start, EndsAt: end}, true},
		"usd limit":       {Request{USDLimit: &usd, StartsAt: start, EndsAt: end}, true},
		"missing limits":  {Request{StartsAt: start, EndsAt: end}, false},
		"reversed bounds": {Request{TokenLimit: &token, StartsAt: end, EndsAt: start}, false},
		"zero token":      {Request{TokenLimit: new(int64), StartsAt: start, EndsAt: end}, false},
	} {
		t.Run(name, func(t *testing.T) { require.Equal(t, tc.want, valid(tc.req)) })
	}
}
