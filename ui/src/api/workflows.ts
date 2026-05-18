import { apiGet, apiPost, apiPatch, apiDelete, apiGetBlob } from './client'
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
  ResumeSessionRequest,
} from '@/types/workflow'

// Export/Import types mirroring be/internal/types/workflow_export_request.go
export interface WorkflowBundleEntry {
  workflow: Record<string, unknown>
  agents: Record<string, unknown>[]
  layer_policies: Record<number, string>
  notifications: Record<string, unknown>[]
}

export interface WorkflowBundle {
  version: string
  exported_at: string
  workflows: WorkflowBundleEntry[]
  python_scripts: Record<string, unknown>[]
}

export interface ImportConflicts {
  workflow_ids: string[]
  python_script_ids: string[]
}

export interface ImportResult {
  workflow_ids: string[]
  python_script_ids: string[]
  skipped: boolean
}

/** Export a single workflow definition */
export async function exportWorkflow(
  id: string
): Promise<{ blob: Blob; filename: string | null }> {
  return apiGetBlob(`/api/v1/workflows/${encodeURIComponent(id)}/export`)
}

/** Export all workflow definitions for the current project */
export async function exportAllWorkflows(): Promise<{ blob: Blob; filename: string | null }> {
  return apiGetBlob('/api/v1/workflows/export')
}

/** Check for import conflicts without committing */
export async function checkImport(bundle: WorkflowBundle): Promise<ImportConflicts> {
  return apiPost<ImportConflicts>('/api/v1/workflows/import/check', bundle)
}

/** Import workflows with the given conflict action */
export async function importWorkflows(
  bundle: WorkflowBundle,
  action: 'overwrite' | 'rename' | 'cancel'
): Promise<ImportResult> {
  return apiPost<ImportResult>('/api/v1/workflows/import', { bundle, action })
}

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

/** Resume a finished agent session (open interactive terminal) */
export async function resumeSession(
  ticketId: string,
  params: ResumeSessionRequest
): Promise<TakeControlResponse> {
  return apiPost<TakeControlResponse>(
    `/api/v1/tickets/${encodeURIComponent(ticketId)}/workflow/resume-session`,
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

/** Kill an interactive session (force-terminate without resuming workflow) */
export async function killInteractive(
  ticketId: string,
  params: ExitInteractiveRequest
): Promise<{ status: string }> {
  return apiPost<{ status: string }>(
    `/api/v1/tickets/${encodeURIComponent(ticketId)}/workflow/kill-interactive`,
    params
  )
}
