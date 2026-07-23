package handoff

import (
	"strings"

	"be/internal/foldfmt"
)

// maxTaskAnchorBytes caps the raw agent_sessions.prompt embedded as the
// Task block — a prompt can be arbitrarily long, but the anchor only needs
// to re-orient the reading model, not reproduce the full spec.
const maxTaskAnchorBytes = 2000

// renderVerified assembles the ## Verified State section from its
// constituent blocks; each empty block is omitted, and the whole section is
// "" when every block is empty.
func renderVerified(hc handoffContext, plan []string, outcome string, verifiedRefs, unverifiedRefs, commands, testLines []string) string {
	var blocks []string

	if strings.TrimSpace(hc.taskAnchor) != "" {
		blocks = append(blocks, "### Task\n"+foldfmt.CapBytes(hc.taskAnchor, maxTaskAnchorBytes))
	}
	if len(plan) > 0 {
		blocks = append(blocks, "### Plan (recorded findings)\n"+strings.Join(plan, "\n"))
	}
	if strings.TrimSpace(outcome) != "" {
		blocks = append(blocks, "### Outcome\n"+outcome)
	}
	if len(verifiedRefs) > 0 {
		blocks = append(blocks, "### Files touched (resolved in this repo)\n"+bulletJoin(verifiedRefs))
	}
	if len(unverifiedRefs) > 0 {
		blocks = append(blocks, "### Unverified references (NOT resolved — do not trust)\n"+bulletJoin(unverifiedRefs))
	}
	if len(commands) > 0 {
		blocks = append(blocks, "### Commands run\n"+bulletJoin(commands))
	}
	if len(testLines) > 0 {
		blocks = append(blocks, "### Test results\n"+bulletJoin(testLines))
	}

	if len(blocks) == 0 {
		return ""
	}
	return "## Verified State\n" + verifiedPreamble + "\n\n" + strings.Join(blocks, "\n\n")
}

func bulletJoin(items []string) string {
	lines := make([]string, len(items))
	for i, it := range items {
		lines[i] = "- " + it
	}
	return strings.Join(lines, "\n")
}
