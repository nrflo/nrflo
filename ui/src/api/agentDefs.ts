import { apiGet, apiPost, apiPatch, apiDelete } from './client'
import type {
  AgentDef,
  AgentDefCreateRequest,
  AgentDefUpdateRequest,
} from '@/types/workflow'

/** List all agent definitions for a workflow */
export async function listAgentDefs(workflowId: string): Promise<AgentDef[]> {
  return apiGet<AgentDef[]>(
    `/api/v1/workflows/${encodeURIComponent(workflowId)}/agents`
  )
}

/** Create a new agent definition */
export async function createAgentDef(
  workflowId: string,
  data: AgentDefCreateRequest
): Promise<AgentDef> {
  return apiPost<AgentDef>(
    `/api/v1/workflows/${encodeURIComponent(workflowId)}/agents`,
    data
  )
}

/** Get a single agent definition */
export async function getAgentDef(
  workflowId: string,
  id: string
): Promise<AgentDef> {
  return apiGet<AgentDef>(
    `/api/v1/workflows/${encodeURIComponent(workflowId)}/agents/${encodeURIComponent(id)}`
  )
}

/** Update an agent definition */
export async function updateAgentDef(
  workflowId: string,
  id: string,
  data: AgentDefUpdateRequest
): Promise<{ status: string }> {
  return apiPatch<{ status: string }>(
    `/api/v1/workflows/${encodeURIComponent(workflowId)}/agents/${encodeURIComponent(id)}`,
    data
  )
}

/** Delete an agent definition */
export async function deleteAgentDef(
  workflowId: string,
  id: string
): Promise<{ status: string }> {
  return apiDelete<{ status: string }>(
    `/api/v1/workflows/${encodeURIComponent(workflowId)}/agents/${encodeURIComponent(id)}`
  )
}
