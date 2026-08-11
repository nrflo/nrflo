package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"be/internal/service"
)

// refineryTierFoldKeys are the per-model-tier fold-start pct keys
// (service/global_settings_refinery_tier.go). Unlike the generic pct knobs
// they are tri-state: unset (null on the wire) falls through to the
// per-model key / built-in tier default / generic key, so the GET surfaces
// nil rather than a default.
var refineryTierFoldKeys = []string{
	service.RefineryFoldStartPctPremiumKey,
	service.RefineryFoldStartPctMidKey,
	service.RefineryFoldStartPctCheapKey,
}

// refineryTierFoldSettings reads the three tier keys as nullable ints.
func refineryTierFoldSettings(svc *service.GlobalSettingsService) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(refineryTierFoldKeys))
	for _, key := range refineryTierFoldKeys {
		val, err := svc.Get(key)
		if err != nil {
			return nil, err
		}
		if parsed, parseErr := strconv.Atoi(val); val != "" && parseErr == nil {
			out[key] = parsed
		} else {
			out[key] = nil
		}
	}
	return out, nil
}

// refineryTierPatchFields carries the tier knobs on the settings PATCH body:
// absent = no-op, null = clear (fall through), number = set.
type refineryTierPatchFields struct {
	RefineryFoldStartPctPremium json.RawMessage `json:"refinery_fold_start_pct_premium"`
	RefineryFoldStartPctMid     json.RawMessage `json:"refinery_fold_start_pct_mid"`
	RefineryFoldStartPctCheap   json.RawMessage `json:"refinery_fold_start_pct_cheap"`
}

// applyRefineryTierSettings persists each present field via
// applyOptionalIntSetting.
func applyRefineryTierSettings(fields refineryTierPatchFields, svc *service.GlobalSettingsService, w http.ResponseWriter) error {
	intFields := []struct {
		raw json.RawMessage
		key string
	}{
		{fields.RefineryFoldStartPctPremium, service.RefineryFoldStartPctPremiumKey},
		{fields.RefineryFoldStartPctMid, service.RefineryFoldStartPctMidKey},
		{fields.RefineryFoldStartPctCheap, service.RefineryFoldStartPctCheapKey},
	}
	for _, f := range intFields {
		if err := applyOptionalIntSetting(svc, f.raw, f.key, w); err != nil {
			return err
		}
	}
	return nil
}
