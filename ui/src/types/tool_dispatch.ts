export type DispatchStatus = 'success' | 'error' | 'pending'

export interface ToolDispatch {
  id: string
  tool_name: string
  status: DispatchStatus
  created_at: string
}
