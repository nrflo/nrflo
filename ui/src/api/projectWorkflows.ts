import { apiGet, apiPost } from './client'
import type {
  ProjectWorkflowResponse,
  ProjectWorkflowRunRequest,
  RunWorkflowResponse,
  RestartAgentRequest,
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
  workflow?: string
): Promise<{ status: string }> {
  return apiPost<{ status: string }>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/workflow/stop`,
    { workflow: workflow ?? '' }
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
