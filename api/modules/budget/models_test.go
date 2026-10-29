package budget

import "testing"

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
