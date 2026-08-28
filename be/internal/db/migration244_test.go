package db

import "testing"

func TestMigration244SeedsGLMAndTierRouting(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var provider, apiModel, efforts, defaultEffort string
	var context, readonly int
	var priceIn, priceOut, priceCacheWrite, priceCacheRead float64
	err = pool.QueryRow(`
		SELECT provider, api_model, api_efforts, api_context, default_effort,
		       read_only, price_in, price_out, price_cache_write, price_cache_read
		FROM models WHERE id = 'glm-5.3-flash'`,
	).Scan(&provider, &apiModel, &efforts, &context, &defaultEffort, &readonly,
		&priceIn, &priceOut, &priceCacheWrite, &priceCacheRead)
	if err != nil {
		t.Fatalf("select GLM model: %v", err)
	}
	if provider != "openrouter" || apiModel != "z-ai/glm-5.3-flash" ||
		efforts != `["low","high","max"]` || context != 1048576 ||
		defaultEffort != "high" || readonly != 1 {
		t.Errorf("unexpected GLM row: provider=%q api=%q efforts=%s context=%d default=%q readonly=%d",
			provider, apiModel, efforts, context, defaultEffort, readonly)
	}
	if priceIn != 0.075 || priceOut != 0.25 || priceCacheWrite != 0.275 || priceCacheRead != 0.015 {
		t.Errorf("unexpected GLM prices: in=%g out=%g write=%g read=%g",
			priceIn, priceOut, priceCacheWrite, priceCacheRead)
	}

	wantEffort := map[int]string{1: "low", 2: "high", 3: "max", 4: "max"}
	for tier, effort := range wantEffort {
		var modelID, mode, gotEffort string
		if err := pool.QueryRow(`
			SELECT model_id, execution_mode, reasoning_effort
			FROM tier_models WHERE tier = ? AND position = 0`, tier,
		).Scan(&modelID, &mode, &gotEffort); err != nil {
			t.Fatalf("select tier %d primary: %v", tier, err)
		}
		if modelID != "glm-5.3-flash" || mode != "api" || gotEffort != effort {
			t.Errorf("tier %d primary = %s/%s/%s, want glm-5.3-flash/api/%s",
				tier, modelID, mode, gotEffort, effort)
		}
	}

	var tierFivePrimary string
	if err := pool.QueryRow(`SELECT model_id FROM tier_models WHERE tier = 5 AND position = 0`).Scan(&tierFivePrimary); err != nil {
		t.Fatalf("select tier 5 primary: %v", err)
	}
	if tierFivePrimary != "opus-5" {
		t.Errorf("tier 5 primary = %q, want opus-5", tierFivePrimary)
	}
	var oldModelCount int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM models WHERE id = 'ox-alpha'`).Scan(&oldModelCount); err != nil {
		t.Fatalf("count ox-alpha: %v", err)
	}
	if oldModelCount != 0 {
		t.Errorf("ox-alpha model rows = %d, want 0", oldModelCount)
	}
}
