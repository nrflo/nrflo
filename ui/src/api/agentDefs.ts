import { apiGet, apiPost, apiPatch, apiDelete } from './client'
import type {
  AgentDef,
  AgentDefCreateRequest,
  AgentDefUpdateRequest,
} from '@/types/workflow'

/** List all agent definitions for a workflow */
export async function listAgentDefs(workflowId: string, project?: string): Promise<AgentDef[]> {
  return apiGet<AgentDef[]>(
    `/api/v1/workflows/${encodeURIComponent(workflowId)}/agents`,
    { project }
  )
}

/** Create a new agent definition */
export async function createAgentDef(
  workflowId: string,
  data: AgentDefCreateRequest,
  project?: string
): Promise<AgentDef> {
  return apiPost<AgentDef>(
    `/api/v1/workflows/${encodeURIComponent(workflowId)}/agents`,
    data,
    { project }
  )
}

/** Get a single agent definition */
export async function getAgentDef(
  workflowId: string,
  id: string,
  project?: string
): Promise<AgentDef> {
  return apiGet<AgentDef>(
    `/api/v1/workflows/${encodeURIComponent(workflowId)}/agents/${encodeURIComponent(id)}`,
    { project }
  )
}

/** Update an agent definition */
export async function updateAgentDef(
  workflowId: string,
  id: string,
  data: AgentDefUpdateRequest,
  project?: string
): Promise<{ status: string }> {
  return apiPatch<{ status: string }>(
    `/api/v1/workflows/${encodeURIComponent(workflowId)}/agents/${encodeURIComponent(id)}`,
    data,
    { project }
  )
}

/** Delete an agent definition */
export async function deleteAgentDef(
  workflowId: string,
  id: string,
  project?: string
): Promise<{ status: string }> {
  return apiDelete<{ status: string }>(
    `/api/v1/workflows/${encodeURIComponent(workflowId)}/agents/${encodeURIComponent(id)}`,
    undefined,
    { project }
  )
}
