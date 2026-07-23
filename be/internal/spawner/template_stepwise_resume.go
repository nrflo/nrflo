package spawner

import (
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/foldfmt"
	"be/internal/handoff"
	"be/internal/service/stepengine"
)

// maxStepwiseResumeBytes caps the whole stepwise ${previous_data} body,
// mirroring handoff's maxHandoffBytes precedent for sizing a relaunch
// document.
const maxStepwiseResumeBytes = 12288

// stepwiseResumeData builds the cursor-driven ${previous_data} body for a
// stepwise def's relaunch: one block per completed step's evidence
// (stepengine.CompletedEvidence — snapshot-declared keys, their stored
// values, and resolved/unresolved path refs), then a fresh refinery slot
// digest under handoff's non-authoritative narrative label when one exists.
// Deliberately does not call handoff.Compose — no generic Verified State, no
// transcript tail; the epic's stepwise invariant is anchor + outline +
// completed-step evidence + current step body + digest narrative, and the
// first three of those are supplied by appendStepwiseBlock, not this
// function. Evidence is keyed by (wfiID, nodeID), not session.
func (s *Spawner) stepwiseResumeData(pool *db.Pool, clk clock.Clock, wfiID, nodeID string, prevStartedAt time.Time) string {
	engine := stepengine.New(pool, clk, nil)
	evidence, err := engine.CompletedEvidence(wfiID, nodeID)
	if err != nil {
		evidence = nil
	}

	var b strings.Builder
	if len(evidence) > 0 {
		b.WriteString("## Completed Steps (verified)\n")
		b.WriteString("Machine-generated from the nrflo database and the working tree — no model involved. AUTHORITATIVE: these steps are already accepted; do not redo them.\n\n")
	}

	for _, ev := range evidence {
		fmt.Fprintf(&b, "### Step %d: %s (step_id=%s)\n", ev.Index+1, ev.Title, ev.StepID)
		for _, f := range ev.Findings {
			value := foldfmt.CapBytes(f.Value, 512)
			fmt.Fprintf(&b, "- %s (%s): %s\n", f.Key, f.Schema, value)
		}
		if len(ev.ResolvedPaths) > 0 {
			b.WriteString("Resolved in this repo:\n")
			for _, p := range ev.ResolvedPaths {
				fmt.Fprintf(&b, "- %s\n", p)
			}
		}
		if len(ev.UnresolvedPaths) > 0 {
			b.WriteString("Unverified references (NOT resolved — do not trust):\n")
			for _, p := range ev.UnresolvedPaths {
				fmt.Fprintf(&b, "- %s\n", p)
			}
		}
		if strings.TrimSpace(ev.Summary) != "" {
			fmt.Fprintf(&b, "Agent-supplied summary (non-authoritative): %s\n", ev.Summary)
		}
		b.WriteString("\n")
	}

	if digest, ok := freshSlotDigest(pool, clk, wfiID, nodeID, prevStartedAt); ok {
		if section := handoff.NarrativeSection(digest); section != "" {
			b.WriteString(section)
			b.WriteString("\n\n")
		}
	}

	return foldfmt.CapBytes(strings.TrimRight(b.String(), "\n"), maxStepwiseResumeBytes)
}
