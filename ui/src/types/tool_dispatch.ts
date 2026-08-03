export type DispatchStatus = 'success' | 'error' | 'pending'

// Audited row shape (mirrors be/internal/model.ToolDispatch after migration
// 000229): every tool dispatch is attributed to its source invoke site and,
// when applicable, the calling session/workflow instance — the
// SessionToolDistribution panel groups on `session_id`. `source`/`duration_ms`/
// `workflow_instance_id` are empty/absent on rows written before the audit.
export interface ToolDispatch {
  id: string
  tool_name: string
  status: DispatchStatus
  created_at: string
  source?: string
  session_id?: string
  session_kind?: string
  duration_ms?: number
  workflow_instance_id?: string
}
