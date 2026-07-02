package spawner

import (
	"context"
	"fmt"
	"os"

	"be/internal/logger"
	"be/internal/repo"
)

// fetchExternalRefs returns the external_id, external_context and launch_depth
// from the workflow instance. Returns ("", "", 0) on nil pool, unresolved wfiID,
// or error.
func (s *Spawner) fetchExternalRefs(projectID, ticketID, workflowName, wfiID string) (string, string, int) {
	pool := s.pool()
	if pool == nil {
		return "", "", 0
	}
	resolvedID := s.resolveWFIID(projectID, ticketID, workflowName, wfiID)
	if resolvedID == "" {
		return "", "", 0
	}
	wi, err := repo.NewWorkflowInstanceRepo(pool, s.config.Clock).Get(resolvedID)
	if err != nil || wi == nil {
		return "", "", 0
	}
	return wi.ExternalID, wi.ExternalContext, wi.LaunchDepth
}

// mergeExtraVars copies base (may be nil) and merges extra on top.
// base is never mutated.
func mergeExtraVars(base, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// buildCLIAgentEnv assembles the environment slice for CLI-mode agent processes.
func (s *Spawner) buildCLIAgentEnv(ctx context.Context, projectID, wfiID, sessionID, spawnToken string, effectiveThreshold, maxContext int, cliStageDir, extID, extCtx string) []string {
	return append(append(filterEnv(os.Environ(), "CLAUDECODE"),
		fmt.Sprintf("NRFLO_PROJECT=%s", projectID),
		fmt.Sprintf("NRF_WORKFLOW_INSTANCE_ID=%s", wfiID),
		fmt.Sprintf("NRF_SESSION_ID=%s", sessionID),
		fmt.Sprintf("NRFLO_AGENT_TOKEN=%s", spawnToken),
		fmt.Sprintf("NRF_TRX=%s", logger.TrxFromContext(ctx)),
		"NRF_SPAWNED=1",
		fmt.Sprintf("NRF_CONTEXT_THRESHOLD=%d", 100-effectiveThreshold),
		fmt.Sprintf("NRF_MAX_CONTEXT=%d", maxContext),
		fmt.Sprintf("NRF_ARTIFACTS_DIR=%s", cliStageDir),
		fmt.Sprintf("NRF_EXTERNAL_ID=%s", extID),
		fmt.Sprintf("NRF_EXTERNAL_CONTEXT=%s", extCtx),
	), s.config.ProjectEnv...)
}
