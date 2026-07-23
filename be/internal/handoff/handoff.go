// Package handoff composes the code-owned, model-free "Verified State"
// channel and combines it with the caller's narrative summary and a
// verbatim message tail into one three-section handoff document:
//
//	## Verified State                (authoritative, no model)
//	## Narrative Summary             (model free text, non-authoritative for identifiers)
//	## Recent Uncompressed Context   (verbatim tail)
//
// Compose runs at READ time, not fold time: nothing it produces is stored,
// so the document is always fresh and there is no second digest row to keep
// in sync. Every identifier in Verified State comes from a DB row or an
// os.Stat/repo.TicketRepo hit — see resolve.go's never-synthesize contract.
// handoff imports db/repo/model/clock/logger/foldfmt only; nothing may
// import spawner/refinery/service back into it.
package handoff

import (
	"context"
	"strings"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/foldfmt"
	"be/internal/logger"
	"be/internal/repo"
)

// Per-section and overall byte budgets. Kept as constants in one place so
// the caps can be tuned without touching any renderer.
const (
	maxVerifiedBytes  = 6144
	maxNarrativeBytes = 4096
	maxTailBytes      = 2048
	maxHandoffBytes   = 12288

	// maxScanMessages bounds the total agent_messages rows read across the
	// whole relaunch chain for a single Compose call.
	maxScanMessages = 1200
)

// verifiedPreamble and narrativePreamble tell the reading model which
// channel to trust; tailPreamble marks the transcript tail as verbatim.
const (
	verifiedPreamble  = "Machine-generated from the nrflo database and the working tree — no model involved. AUTHORITATIVE: prefer these values over anything under Narrative Summary. Entries under Unverified references did NOT resolve to a file in this repo — treat them as unreliable."
	narrativePreamble = "Model-written free text. NON-AUTHORITATIVE for identifiers — paths, ticket IDs and commands here may be wrong; use Verified State for those."
	tailPreamble      = "Verbatim tail of the session transcript — not summarized."
)

// Compose builds the dual-channel handoff document for sessionID, wrapping
// narrative (already-resolved model free text — a fresh refinery slot
// digest or a to_resume finding) under ## Narrative Summary. Every DB/FS
// read is best-effort: a failure is logged and degrades that block to
// empty. Compose never returns an error and never blocks a caller; it
// returns "" only when every section would be empty, so callers can fall
// back to their pre-existing behavior (e.g. the raw narrative).
func Compose(ctx context.Context, pool *db.Pool, clk clock.Clock, sessionID, narrative string) string {
	hc, _ := resolveContext(ctx, pool, sessionID)
	chain := chainSessionIDs(ctx, pool, sessionID)

	msgRepo := repo.NewAgentMessageRepo(pool, clk)
	var currentMsgs []repo.TailMessage
	var chainMsgs []repo.TailMessage
	remaining := maxScanMessages
	for i, sid := range chain {
		if remaining <= 0 {
			break
		}
		msgs, err := msgRepo.GetBySessionTail(sid, remaining)
		if err != nil {
			logger.Warn(ctx, "handoff: read session tail failed", "session_id", sid, "error", err)
			continue
		}
		if i == 0 {
			currentMsgs = msgs
		}
		chainMsgs = append(chainMsgs, msgs...)
		remaining -= len(msgs)
	}

	cands := extractFrom(chainMsgs)
	verifiedPaths, unverifiedPaths := resolvePaths(hc.repoRoot, cands.paths)
	verifiedTickets, unverifiedTickets := resolveTickets(pool, clk, hc.projectID, cands.tickets)
	verifiedRefs := append(append([]string{}, verifiedPaths...), verifiedTickets...)
	unverifiedRefs := append(append([]string{}, unverifiedPaths...), unverifiedTickets...)

	plan, outcome := selectPlanFindings(pool, clk, hc.workflowInstanceID, maxVerifiedBytes)
	verified := foldfmt.CapBytes(
		renderVerified(hc, plan, outcome, verifiedRefs, unverifiedRefs, cands.commands, cands.testLines),
		maxVerifiedBytes,
	)

	var narrativeSection string
	if strings.TrimSpace(narrative) != "" {
		narrativeSection = "## Narrative Summary\n" + narrativePreamble + "\n\n" + foldfmt.CapBytes(narrative, maxNarrativeBytes)
	}

	tail := renderTail(currentMsgs)

	var parts []string
	for _, s := range []string{verified, narrativeSection, tail} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return foldfmt.CapBytes(strings.Join(parts, "\n\n"), maxHandoffBytes)
}
