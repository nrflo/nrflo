package service

import (
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

// setupTierModelTestEnv creates an isolated DB + TierModelService for tests,
// mirroring setupSysAgentDefTestEnv's per-test template-DB copy pattern.
func setupTierModelTestEnv(t *testing.T) (*TierModelService, *ModelService, func()) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tier_model_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to open pool: %v", err)
	}
	modelSvc := NewModelService(pool, clock.Real())
	svc := NewTierModelService(pool, clock.Real(), modelSvc)
	return svc, modelSvc, func() { pool.Close() }
}

// --- List ---

// TestTierModel_List_OrderedByTierThenPosition verifies List returns every
// row ordered by (tier, position) ascending, spanning the seeded tier1/tier4
// chains (migration 000195) plus a freshly-set tier.
func TestTierModel_List_OrderedByTierThenPosition(t *testing.T) {
	t.Parallel()
	svc, _, cleanup := setupTierModelTestEnv(t)
	defer cleanup()

	if err := svc.SetTierChain(2, []types.TierChainEntry{
		{ExecutionMode: "api", ModelID: "haiku-4-5"},
	}); err != nil {
		t.Fatalf("SetTierChain(2): %v", err)
	}

	rows, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var lastTier, lastPos int
	seenTiers := map[int]bool{}
	for i, r := range rows {
		if i > 0 {
			if r.Tier < lastTier || (r.Tier == lastTier && r.Position < lastPos) {
				t.Fatalf("row %d out of order: %+v after tier=%d pos=%d", i, r, lastTier, lastPos)
			}
		}
		lastTier, lastPos = r.Tier, r.Position
		seenTiers[r.Tier] = true
	}
	for _, want := range []int{1, 2, 4} {
		if !seenTiers[want] {
			t.Errorf("List missing tier=%d rows", want)
		}
	}
}

// --- SetTierChain: replace + renumber ---

// TestTierModel_SetTierChain_ReplacesAndRenumbers verifies SetTierChain
// deletes any prior rows for the tier and inserts the new entries with
// position = array index starting at 0.
func TestTierModel_SetTierChain_ReplacesAndRenumbers(t *testing.T) {
	t.Parallel()
	svc, _, cleanup := setupTierModelTestEnv(t)
	defer cleanup()

	// Tier 1 arrives pre-seeded with 2 rows (migration 000195).
	if err := svc.SetTierChain(1, []types.TierChainEntry{
		{ExecutionMode: "api", ModelID: "opus-4-7"},
	}); err != nil {
		t.Fatalf("SetTierChain: %v", err)
	}

	rows, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var tier1 []struct {
		pos     int
		modelID string
	}
	for _, r := range rows {
		if r.Tier == 1 {
			tier1 = append(tier1, struct {
				pos     int
				modelID string
			}{r.Position, r.ModelID})
		}
	}
	if len(tier1) != 1 {
		t.Fatalf("tier1 rows = %d, want 1 (old seeded rows must be replaced)", len(tier1))
	}
	if tier1[0].pos != 0 || tier1[0].modelID != "opus-4-7" {
		t.Errorf("tier1[0] = %+v, want pos=0 model=opus-4-7", tier1[0])
	}
}

// TestTierModel_SetTierChain_ReorderPersistsNewOrder verifies re-calling
// SetTierChain with entries in a different order renumbers positions to
// match the new order (reorder = fallback priority).
func TestTierModel_SetTierChain_ReorderPersistsNewOrder(t *testing.T) {
	t.Parallel()
	svc, _, cleanup := setupTierModelTestEnv(t)
	defer cleanup()

	if err := svc.SetTierChain(3, []types.TierChainEntry{
		{ExecutionMode: "api", ModelID: "haiku-4-5"},
		{ExecutionMode: "api", ModelID: "opus-4-7"},
	}); err != nil {
		t.Fatalf("initial SetTierChain: %v", err)
	}

	// Reorder: opus first, haiku second.
	if err := svc.SetTierChain(3, []types.TierChainEntry{
		{ExecutionMode: "api", ModelID: "opus-4-7"},
		{ExecutionMode: "api", ModelID: "haiku-4-5"},
	}); err != nil {
		t.Fatalf("reorder SetTierChain: %v", err)
	}

	rows, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var tier3 []struct {
		pos     int
		modelID string
	}
	for _, r := range rows {
		if r.Tier == 3 {
			tier3 = append(tier3, struct {
				pos     int
				modelID string
			}{r.Position, r.ModelID})
		}
	}
	if len(tier3) != 2 {
		t.Fatalf("tier3 rows = %d, want 2", len(tier3))
	}
	if tier3[0].pos != 0 || tier3[0].modelID != "opus-4-7" {
		t.Errorf("tier3[0] = %+v, want pos=0 model=opus-4-7", tier3[0])
	}
	if tier3[1].pos != 1 || tier3[1].modelID != "haiku-4-5" {
		t.Errorf("tier3[1] = %+v, want pos=1 model=haiku-4-5", tier3[1])
	}
}

// TestTierModel_SetTierChain_EmptyEntriesClearsTier verifies an empty
// Entries slice clears every row for that tier.
func TestTierModel_SetTierChain_EmptyEntriesClearsTier(t *testing.T) {
	t.Parallel()
	svc, _, cleanup := setupTierModelTestEnv(t)
	defer cleanup()

	// Tier 4 is pre-seeded with 2 rows.
	if err := svc.SetTierChain(4, []types.TierChainEntry{}); err != nil {
		t.Fatalf("SetTierChain(empty): %v", err)
	}

	rows, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, r := range rows {
		if r.Tier == 4 {
			t.Errorf("tier4 row survived clear: %+v", r)
		}
	}
}
