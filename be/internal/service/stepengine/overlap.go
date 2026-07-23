package stepengine

import (
	"encoding/json"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"be/internal/model"
)

// checkPathOverlap evaluates a step's optional path_overlap gate against the
// node's already-loaded findings: it collects the path-bearing values named
// by the Left and Right key groups (missing or unparsable keys contribute no
// paths — they are reported as missing/invalid evidence elsewhere) and
// returns the sorted, deduped set of normalized paths claimed by both sides.
// A nil rule always returns nil.
func checkPathOverlap(findings map[string]json.RawMessage, rule *model.PathOverlap) []string {
	if rule == nil {
		return nil
	}
	left := collectNormalizedPaths(findings, rule.Left)
	right := collectNormalizedPaths(findings, rule.Right)

	seen := make(map[string]bool, len(left))
	for p := range left {
		seen[p] = true
	}
	var offenders []string
	for p := range right {
		if seen[p] {
			offenders = append(offenders, p)
		}
	}
	sort.Strings(offenders)
	return offenders
}

// collectNormalizedPaths gathers the json_array_path_change "path" values
// found under the given findings keys into a deduped set, normalized with
// path.Clean(filepath.ToSlash(...)) so "./x" and "x" compare equal.
func collectNormalizedPaths(findings map[string]json.RawMessage, keys []string) map[string]bool {
	out := make(map[string]bool)
	for _, key := range keys {
		raw, ok := findings[key]
		if !ok {
			continue
		}
		paths, err := validateJSONArrayPathChange(unwrapOnce(raw))
		if err != nil {
			continue
		}
		for _, p := range paths {
			norm := path.Clean(filepath.ToSlash(strings.TrimSpace(p)))
			if norm == "" || norm == "." {
				continue
			}
			out[norm] = true
		}
	}
	return out
}
