package service

import (
	"strconv"
	"strings"

	"be/internal/db"
)

// Stepwise evidence-rejection cap config key/default and the result_reason
// constant recorded when a stepwise session force-fails at the cap. Lives in
// service/ (not stepengine) so stepengine keeps its import hygiene (never
// imports service) while tools_builtin can still read the config.
const (
	StepRejectionCapKey     = "stepwise_rejection_cap"
	DefaultStepRejectionCap = 5

	// ResultReasonStepEvidenceExhausted is agent_sessions.result_reason when a
	// stepwise agent's complete_step call is rejected at the rejection cap.
	ResultReasonStepEvidenceExhausted = "step_evidence_exhausted"

	// ResultReasonStepsIncomplete is agent_sessions.result_reason when a
	// stepwise agent signals pass while the server-owned cursor is still
	// short of its last step.
	ResultReasonStepsIncomplete = "steps_incomplete"

	// FindingKeyStepsIncomplete is the session finding key written alongside
	// ResultReasonStepsIncomplete, carried to the retry session by
	// copyFindingsForContinuation.
	FindingKeyStepsIncomplete = "steps_incomplete"
)

// StepRejectionCap reads the per-step evidence-rejection cap from the config
// KV: project override first, then global, falling back to
// DefaultStepRejectionCap when unset or unparsable. Cloned from
// SubworkflowCap's precedence (subworkflow.go:26).
func StepRejectionCap(pool *db.Pool, projectID string) int {
	raw, err := pool.GetProjectConfig(projectID, StepRejectionCapKey)
	if err != nil || raw == "" {
		raw, err = pool.GetConfig(StepRejectionCapKey)
		if err != nil || raw == "" {
			return DefaultStepRejectionCap
		}
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return DefaultStepRejectionCap
	}
	return n
}
