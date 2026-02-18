import { apiGet, apiPost, apiPatch, apiDelete } from './client'
import type {
  WorkflowDef,
  WorkflowDefSummary,
  WorkflowDefCreateRequest,
  WorkflowDefUpdateRequest,
  RunWorkflowRequest,
  RunWorkflowResponse,
  StopWorkflowRequest,
  RestartAgentRequest,
  TakeControlRequest,
  TakeControlResponse,
  ExitInteractiveRequest,
} from '@/types/workflow'

/** List all workflow definitions for current project */
export async function listWorkflowDefs(): Promise<Record<string, WorkflowDefSummary>> {
  return apiGet<Record<string, WorkflowDefSummary>>('/api/v1/workflows')
}

/** Create a new workflow definition */
export async function createWorkflowDef(
  data: WorkflowDefCreateRequest
): Promise<WorkflowDef> {
  return apiPost<WorkflowDef>('/api/v1/workflows', data)
}

/** Get a single workflow definition */
export async function getWorkflowDef(id: string): Promise<WorkflowDef> {
  return apiGet<WorkflowDef>(`/api/v1/workflows/${encodeURIComponent(id)}`)
}

/** Update a workflow definition */
export async function updateWorkflowDef(
  id: string,
  data: WorkflowDefUpdateRequest
): Promise<{ status: string }> {
  return apiPatch<{ status: string }>(
    `/api/v1/workflows/${encodeURIComponent(id)}`,
    data
  )
}

/** Delete a workflow definition */
export async function deleteWorkflowDef(
  id: string
): Promise<{ status: string }> {
  return apiDelete<{ status: string }>(
    `/api/v1/workflows/${encodeURIComponent(id)}`
  )
}

/** Start an orchestrated workflow run */
export async function runWorkflow(
  ticketId: string,
  params: RunWorkflowRequest
): Promise<RunWorkflowResponse> {
  return apiPost<RunWorkflowResponse>(
    `/api/v1/tickets/${encodeURIComponent(ticketId)}/workflow/run`,
    params
  )
}

/** Stop a running orchestrated workflow */
export async function stopWorkflow(
  ticketId: string,
  params?: StopWorkflowRequest
): Promise<{ status: string }> {
  return apiPost<{ status: string }>(
    `/api/v1/tickets/${encodeURIComponent(ticketId)}/workflow/stop`,
    params ?? {}
  )
}

/** Restart a running agent (save context, relaunch) */
export async function restartAgent(
  ticketId: string,
  params: RestartAgentRequest
): Promise<{ status: string }> {
  return apiPost<{ status: string }>(
    `/api/v1/tickets/${encodeURIComponent(ticketId)}/workflow/restart`,
    params
  )
}

/** Retry a failed workflow from the failed layer */
export async function retryFailedAgent(
  ticketId: string,
  params: RestartAgentRequest
): Promise<{ status: string }> {
  return apiPost<{ status: string }>(
    `/api/v1/tickets/${encodeURIComponent(ticketId)}/workflow/retry-failed`,
    params
  )
}

/** Take interactive control of a running agent */
export async function takeControl(
  ticketId: string,
  params: TakeControlRequest
): Promise<TakeControlResponse> {
  return apiPost<TakeControlResponse>(
    `/api/v1/tickets/${encodeURIComponent(ticketId)}/workflow/take-control`,
    params
  )
}

/** Exit interactive session and resume workflow */
export async function exitInteractive(
  ticketId: string,
  params: ExitInteractiveRequest
): Promise<{ status: string }> {
  return apiPost<{ status: string }>(
    `/api/v1/tickets/${encodeURIComponent(ticketId)}/workflow/exit-interactive`,
    params
  )
}
