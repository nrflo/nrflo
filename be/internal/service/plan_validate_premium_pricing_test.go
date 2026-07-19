package service

import (
	"testing"

	"be/internal/clock"
)

// TestPlanModelTierClass_PricingOverridesNameClass asserts the DB-seeded
// per-MTok pricing wins over the name-class fallback: a model whose id would
// name-classify as cheap (haiku) but whose price_in is UPDATEd to a premium
// value must classify premium, and vice versa for a name that would
// name-classify premium (opus) but is priced mid.
func TestPlanModelTierClass_PricingOverridesNameClass(t *testing.T) {
	t.Parallel()
	pool, _, _ := setupPlanValidateEnv(t)
	modelSvc := NewModelService(pool, clock.Real())

	mustExec(t, pool, `UPDATE models SET price_in = 10 WHERE id = 'haiku-4-5'`)
	haiku, err := modelSvc.Get("haiku-4-5")
	if err != nil {
		t.Fatalf("Get(haiku-4-5): %v", err)
	}
	if got := PlanModelTierClass(haiku); got != ModelTierPremium {
		t.Errorf("PlanModelTierClass(haiku-4-5, price_in=10) = %v, want premium (pricing overrides the cheap name-class)", got)
	}

	mustExec(t, pool, `UPDATE models SET price_in = 3 WHERE id = 'opus-4-8'`)
	opus, err := modelSvc.Get("opus-4-8")
	if err != nil {
		t.Fatalf("Get(opus-4-8): %v", err)
	}
	if got := PlanModelTierClass(opus); got != ModelTierMid {
		t.Errorf("PlanModelTierClass(opus-4-8, price_in=3) = %v, want mid (pricing overrides the premium name-class)", got)
	}
}
