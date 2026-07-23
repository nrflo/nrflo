package handoff

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/foldfmt"
	"be/internal/repo"
)

// maxFindingValueBytes caps a single plan-finding value before it is
// embedded in the Plan block.
const maxFindingValueBytes = 1500

// planKeySuffixes are the finding-key suffixes selectPlanFindings treats as
// plan content, as opposed to arbitrary agent scratch findings.
var planKeySuffixes = []string{
	"_files_to_create", "_files_to_modify", "_files_changed",
	"_implementation_steps", "_plan_summary", "_changes_summary",
	"_patterns_to_follow", "_testing_notes", "_risks",
}

type planEntry struct {
	agentType string
	key       string
	value     string
}

// selectPlanFindings reads session findings recorded across the workflow
// instance and deterministically selects the Plan + Outcome blocks:
// workflow_final_result becomes the Outcome, `_`-prefixed internal keys are
// dropped, and keys whose suffix is in planKeySuffixes are kept as plan
// lines, emitted in a fixed (agent_type, key) sorted order so two Compose
// calls over the same DB state produce byte-identical output. Plan-finding
// paths are rendered verbatim — NOT run through resolve.go, since e.g.
// files_to_create legitimately does not exist yet. Stops once budget is
// spent, leaving an "(N further plan findings omitted)" marker.
func selectPlanFindings(pool *db.Pool, clk clock.Clock, wfiID string, budget int) (planLines []string, outcome string) {
	findingRepo := repo.NewFindingRepo(pool, clk)
	grouped, err := findingRepo.ListByWorkflowInstance(wfiID)
	if err != nil {
		return nil, ""
	}

	var entries []planEntry
	for agentKey, findings := range grouped {
		agentType := agentKey
		if idx := strings.IndexByte(agentKey, ':'); idx >= 0 {
			agentType = agentKey[:idx]
		}
		for key, raw := range findings {
			if strings.HasPrefix(key, "_") {
				continue
			}
			if key == "workflow_final_result" {
				outcome = decodeFindingValue(raw)
				continue
			}
			if !hasPlanSuffix(key) {
				continue
			}
			entries = append(entries, planEntry{agentType: agentType, key: key, value: decodeFindingValue(raw)})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].agentType != entries[j].agentType {
			return entries[i].agentType < entries[j].agentType
		}
		return entries[i].key < entries[j].key
	})

	spent := 0
	for i, e := range entries {
		val := foldfmt.CapBytes(e.value, maxFindingValueBytes)
		if len(val) < len(e.value) {
			val += " [truncated]"
		}
		line := fmt.Sprintf("[%s] %s: %s", e.agentType, e.key, val)
		if spent+len(line) > budget && len(planLines) > 0 {
			planLines = append(planLines, fmt.Sprintf("(%d further plan findings omitted)", len(entries)-i))
			return planLines, outcome
		}
		planLines = append(planLines, line)
		spent += len(line)
	}
	return planLines, outcome
}

func hasPlanSuffix(key string) bool {
	for _, suf := range planKeySuffixes {
		if strings.HasSuffix(key, suf) {
			return true
		}
	}
	return false
}

// decodeFindingValue unmarshals a JSON string value to plain text so it
// renders without surrounding quotes; any other JSON shape (array, object,
// number) renders as its compact JSON text.
func decodeFindingValue(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}
