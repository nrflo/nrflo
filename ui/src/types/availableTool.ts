// AvailableTool is one entry from GET /api/v1/available-tools — the source for
// the per-agent tools picker. `mandatory` marks the always-granted baseline
// tools (agent_* lifecycle + findings_add) the spawner grants to
// socket-completion (CLI/api-via-cli) agents regardless of the tools CSV.
export interface AvailableTool {
  name: string
  description: string
  source: 'builtin' | 'python'
  mandatory: boolean
}
