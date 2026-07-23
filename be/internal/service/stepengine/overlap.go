package stepengine

import (
	"encoding/json"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"be/internal/handoff"
	"be/internal/model"
)

// checkPathOverlap evaluates a step's optional path_overlap gate against the
// node's already-loaded findings: it collects the path-bearing values named
// by the Left and Right key groups (missing or unparsable keys contribute no
// paths — they are reported as missing/invalid evidence elsewhere) and
// returns the sorted, deduped set of paths claimed by both sides. Each path is
// resolved against the worktree root so a bare basename and its unique full
// path compare equal; unresolved/ambiguous paths fall back to literal
// normalized comparison (an empty root leaves everything literal). A nil rule
// always returns nil.
func checkPathOverlap(findings map[string]json.RawMessage, rule *model.PathOverlap, root string) []string {
	if rule == nil {
		return nil
	}
	left := collectNormalizedPaths(findings, rule.Left, root)
	right := collectNormalizedPaths(findings, rule.Right, root)

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
// found under the given findings keys into a deduped comparison set. Each
// value is resolved against root (handoff.ResolvePathCandidates) to its
// canonical repo-relative form when it uniquely matches a worktree file, so a
// bare basename and its full path collapse to the same key; anything the
// resolver leaves unresolved/ambiguous (including every value when root is
// empty) falls back to path.Clean(filepath.ToSlash(...)) literal comparison.
func collectNormalizedPaths(findings map[string]json.RawMessage, keys []string, root string) map[string]bool {
	var raws []string
	for _, key := range keys {
		raw, ok := findings[key]
		if !ok {
			continue
		}
		paths, err := validateJSONArrayPathChange(unwrapOnce(raw))
		if err != nil {
			continue
		}
		raws = append(raws, paths...)
	}

	out := make(map[string]bool)
	for _, pr := range handoff.ResolvePathCandidates(root, raws) {
		key := pr.Resolved
		if pr.Status != handoff.PathResolved {
			key = path.Clean(filepath.ToSlash(strings.TrimSpace(pr.Candidate)))
		}
		if key == "" || key == "." {
			continue
		}
		out[key] = true
	}
	return out
}
