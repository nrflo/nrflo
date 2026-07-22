package service

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// effortRank orders levels weakest→strongest; it also defines the global
// effort enum. Per-model capability lives in the model row's
// supported_efforts JSON column (migration 000166), not in code. "none" is
// the weakest level (rank -1, below "low") and is only reachable in
// practice on an ollama_native custom provider's api_efforts — model CRUD
// (model.go/model_update.go) gates it there so it never leaks onto a
// non-Ollama model row.
var effortRank = map[string]int{
	"none": -1, "low": 0, "medium": 1, "high": 2, "xhigh": 3, "max": 4, "ultra": 5,
}

// ValidateEffortAllowed checks a reasoning-effort value against a model
// row's supported_efforts list. "" always passes — it means "inherit the
// row/provider default". Exported so the spawner and the console chat
// resolver re-validate def-level/create-time overrides against the model
// row with the same rule the registry CRUD uses.
func ValidateEffortAllowed(effort string, supported []string) error {
	if effort == "" {
		return nil
	}
	if _, ok := effortRank[effort]; !ok {
		return fmt.Errorf("invalid reasoning_effort %q: must be one of low, medium, high, xhigh, max, ultra", effort)
	}
	if !slices.Contains(supported, effort) {
		if len(supported) == 0 {
			return fmt.Errorf("reasoning_effort %q: this model does not support effort selection", effort)
		}
		return fmt.Errorf("reasoning_effort %q is not supported by this model (supported: %s)", effort, strings.Join(supported, ", "))
	}
	return nil
}

// NormalizeSupportedEfforts validates and sorts a supported_efforts list
// weakest→strongest, dropping duplicates. Used by the cli/api model CRUD.
func NormalizeSupportedEfforts(efforts []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(efforts))
	for _, e := range efforts {
		if _, ok := effortRank[e]; !ok {
			return nil, fmt.Errorf("invalid supported_efforts entry %q: must be one of low, medium, high, xhigh, max, ultra", e)
		}
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return effortRank[out[i]] < effortRank[out[j]] })
	return out, nil
}
