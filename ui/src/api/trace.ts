import { apiGet } from './client'
import type { WorkflowTraceResponse } from '@/components/workflow/Trace/types'

export function getWorkflowTrace(wfiId: string): Promise<WorkflowTraceResponse> {
  return apiGet<WorkflowTraceResponse>(
    `/api/v1/workflow-instances/${encodeURIComponent(wfiId)}/trace`
  )
}
