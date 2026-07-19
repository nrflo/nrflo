package model

import "testing"

func floatPtr(v float64) *float64 { return &v }

// TestModelPriceClass_ThresholdMatrix pins the >=5 premium, >=2 mid, else
// cheap thresholds against the ticket's seeded per-model prices, plus the
// exact boundary values.
func TestModelPriceClass_ThresholdMatrix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		priceIn *float64
		want    PriceTier
		wantOK  bool
	}{
		{"fable-5 seeded price_in=10", floatPtr(10), PricePremium, true},
		{"opus family seeded price_in=5", floatPtr(5), PricePremium, true},
		{"gpt-5.6-sol seeded price_in=5", floatPtr(5), PricePremium, true},
		{"sonnet-5 seeded price_in=3", floatPtr(3), PriceMid, true},
		{"gpt-5.6-terra seeded price_in=2.5", floatPtr(2.5), PriceMid, true},
		{"haiku-4-5 seeded price_in=1", floatPtr(1), PriceCheap, true},
		{"gpt-5.6-luna seeded price_in=1", floatPtr(1), PriceCheap, true},
		{"boundary: exactly 5 is premium (>=5)", floatPtr(5.0), PricePremium, true},
		{"boundary: just under 5 is mid", floatPtr(4.99), PriceMid, true},
		{"boundary: exactly 2 is mid (>=2)", floatPtr(2.0), PriceMid, true},
		{"boundary: just under 2 is cheap", floatPtr(1.99), PriceCheap, true},
		{"boundary: zero is cheap", floatPtr(0), PriceCheap, true},
		{"NULL price_in falls back (ok=false)", nil, PriceCheap, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := &Model{PriceIn: tc.priceIn}
			gotTier, gotOK := m.PriceClass()
			if gotOK != tc.wantOK {
				t.Errorf("PriceClass() ok = %v, want %v", gotOK, tc.wantOK)
			}
			if gotTier != tc.want {
				t.Errorf("PriceClass() tier = %v, want %v", gotTier, tc.want)
			}
		})
	}
}

// TestModelPriceClass_NilModelPriceInDoesNotPanic guards the nil-pointer
// dereference on a zero-value Model (no pricing seeded at all).
func TestModelPriceClass_NilModelPriceInDoesNotPanic(t *testing.T) {
	t.Parallel()
	m := &Model{ID: "unseeded-model"}
	tier, ok := m.PriceClass()
	if ok {
		t.Errorf("PriceClass() ok = true for a model with nil PriceIn, want false")
	}
	if tier != PriceCheap {
		t.Errorf("PriceClass() tier = %v, want PriceCheap (zero value) when ok=false", tier)
	}
}
