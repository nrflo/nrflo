// AvailableTool is one entry from GET /api/v1/available-tools — the source for
// the per-agent tools picker. `mandatory` marks lifecycle tools the spawner
// always grants to socket-completion (CLI/api-via-cli) agents.
export interface AvailableTool {
  name: string
  description: string
  source: 'builtin' | 'python'
  mandatory: boolean
}
