package model

// PriceTier is a model's cost class, derived from its price_in (USD per
// million input tokens).
type PriceTier int

const (
	PriceCheap PriceTier = iota
	PriceMid
	PricePremium
)

// PriceClass maps m.PriceIn to a PriceTier: >=5 premium, >=2 mid, else
// cheap. ok=false when PriceIn is nil (no seeded pricing) — callers fall
// back to name-based classification.
func (m *Model) PriceClass() (PriceTier, bool) {
	if m.PriceIn == nil {
		return PriceCheap, false
	}
	switch {
	case *m.PriceIn >= 5:
		return PricePremium, true
	case *m.PriceIn >= 2:
		return PriceMid, true
	default:
		return PriceCheap, true
	}
}
