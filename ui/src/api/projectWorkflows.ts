import { apiGet, apiPost, apiDelete } from './client'
import type {
  ProjectWorkflowResponse,
  ProjectWorkflowRunRequest,
  RunWorkflowResponse,
  RestartAgentRequest,
  ProjectAgentSessionsResponse,
  TakeControlRequest,
  TakeControlResponse,
  ExitInteractiveRequest,
  ResumeSessionRequest,
} from '@/types/workflow'

/** Get workflow state for a project (all project-scoped instances) */
export async function getProjectWorkflow(
  projectId: string,
  workflow?: string
): Promise<ProjectWorkflowResponse> {
  const params = workflow ? `?workflow=${encodeURIComponent(workflow)}` : ''
  return apiGet<ProjectWorkflowResponse>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/workflow${params}`
  )
}

/** Start an orchestrated project-scoped workflow run */
export async function runProjectWorkflow(
  projectId: string,
  params: ProjectWorkflowRunRequest
): Promise<RunWorkflowResponse> {
  return apiPost<RunWorkflowResponse>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/workflow/run`,
    params
  )
}

/** Stop a running project-scoped workflow */
export async function stopProjectWorkflow(
  projectId: string,
  params: { workflow?: string; instance_id?: string }
): Promise<{ status: string }> {
  return apiPost<{ status: string }>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/workflow/stop`,
    params
  )
}

/** Get agent sessions for a project (all project-scoped instances) */
export async function getProjectAgentSessions(
  projectId: string
): Promise<ProjectAgentSessionsResponse> {
  return apiGet<ProjectAgentSessionsResponse>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/agents`
  )
}

/** Restart an agent in a project-scoped workflow */
export async function restartProjectAgent(
  projectId: string,
  params: RestartAgentRequest
): Promise<{ status: string }> {
  return apiPost<{ status: string }>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/workflow/restart`,
    params
  )
}

/** Retry a failed workflow from the failed layer (project-scoped) */
export async function retryFailedProjectAgent(
  projectId: string,
  params: RestartAgentRequest
): Promise<{ status: string }> {
  return apiPost<{ status: string }>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/workflow/retry-failed`,
    params
  )
}

/** Take interactive control of a running agent (project-scoped) */
export async function takeControlProject(
  projectId: string,
  params: TakeControlRequest
): Promise<TakeControlResponse> {
  return apiPost<TakeControlResponse>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/workflow/take-control`,
    params
  )
}

/** Resume a finished agent session (project-scoped) */
export async function resumeSessionProject(
  projectId: string,
  params: ResumeSessionRequest
): Promise<TakeControlResponse> {
  return apiPost<TakeControlResponse>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/workflow/resume-session`,
    params
  )
}

/** Delete a completed/failed project workflow instance */
export async function deleteProjectWorkflowInstance(
  projectId: string,
  instanceId: string
): Promise<{ message: string }> {
  return apiDelete<{ message: string }>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/workflow/${encodeURIComponent(instanceId)}`
  )
}

/** Get project findings */
export async function getProjectFindings(
  projectId: string
): Promise<Record<string, unknown>> {
  return apiGet<Record<string, unknown>>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/findings`
  )
}

/** Exit interactive session (project-scoped) */
export async function exitInteractiveProject(
  projectId: string,
  params: ExitInteractiveRequest
): Promise<{ status: string }> {
  return apiPost<{ status: string }>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/workflow/exit-interactive`,
    params
  )
}
