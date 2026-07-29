// Mirrors be/internal/model/system_agent_run.go — keep field names/types in
// sync with the Go json tags. fallback_from/tokens_json arrive as
// json.RawMessage on the wire, i.e. already-parsed JSON in the response
// body (not strings) once decoded here.

export type SystemAgentRunKind = 'agent_session' | 'refinery_fold' | 'step_rotation'

// Mirrors be/internal/service/tier_chain_resolve.go's AgentChainEntry.
export interface FallbackEntry {
  provider: string
  execution_mode: string
  model_id: string
  reasoning_effort: string
  tier: number
}

export interface SessionTokens {
  input_tokens?: number
  output_tokens?: number
  cache_read_tokens?: number
  cache_write_tokens?: number
}

export interface SystemAgentRun {
  kind: SystemAgentRunKind
  session_id: string
  agent_type?: string
  tier?: number | null
  resolved_provider?: string
  resolved_execution_mode?: string
  resolved_effort?: string
  chain_position?: number
  fallback_from?: FallbackEntry[]
  model_id?: string
  tokens_json?: SessionTokens
  cost_estimate?: number | null
  status?: string
  result?: string
  workflow_instance_id?: string
  ticket_id?: string
  node_id?: string
  project_id?: string
  prompt_tokens?: number
  output_tokens?: number
  error?: string
  fold_count?: number
  step_id?: string
  created_at: string
  delegation_id?: string
  caller_session_id?: string
  delegate_tier?: string
  fanout?: number
  delegation_status?: string
}

export interface SystemAgentRunsResponse {
  items: SystemAgentRun[]
  limit: number
}
