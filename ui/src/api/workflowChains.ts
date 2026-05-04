import { apiGet, apiPost, apiPatch, apiDelete } from './client'
import type {
  WorkflowChain,
  WorkflowChainWithSteps,
  WorkflowChainCreateRequest,
  WorkflowChainUpdateRequest,
  WorkflowChainStepRequest,
  WorkflowChainStepUpdateRequest,
  ReorderStepsRequest,
} from '@/types/workflowChain'

export async function listChains(): Promise<WorkflowChain[]> {
  return apiGet<WorkflowChain[]>('/api/v1/workflow-chains')
}

export async function getChain(id: string): Promise<WorkflowChainWithSteps> {
  return apiGet<WorkflowChainWithSteps>(`/api/v1/workflow-chains/${encodeURIComponent(id)}`)
}

export async function createChain(data: WorkflowChainCreateRequest): Promise<WorkflowChainWithSteps> {
  return apiPost<WorkflowChainWithSteps>('/api/v1/workflow-chains', data)
}

export async function updateChain(
  id: string,
  data: WorkflowChainUpdateRequest
): Promise<WorkflowChainWithSteps> {
  return apiPatch<WorkflowChainWithSteps>(`/api/v1/workflow-chains/${encodeURIComponent(id)}`, data)
}

export async function deleteChain(id: string): Promise<void> {
  return apiDelete<void>(`/api/v1/workflow-chains/${encodeURIComponent(id)}`)
}

export async function appendStep(
  chainId: string,
  data: WorkflowChainStepRequest
): Promise<WorkflowChainWithSteps> {
  return apiPost<WorkflowChainWithSteps>(
    `/api/v1/workflow-chains/${encodeURIComponent(chainId)}/steps`,
    data
  )
}

export async function updateStep(
  chainId: string,
  stepId: string,
  data: WorkflowChainStepUpdateRequest
): Promise<WorkflowChainWithSteps> {
  return apiPatch<WorkflowChainWithSteps>(
    `/api/v1/workflow-chains/${encodeURIComponent(chainId)}/steps/${encodeURIComponent(stepId)}`,
    data
  )
}

export async function deleteStep(
  chainId: string,
  stepId: string
): Promise<WorkflowChainWithSteps> {
  return apiDelete<WorkflowChainWithSteps>(
    `/api/v1/workflow-chains/${encodeURIComponent(chainId)}/steps/${encodeURIComponent(stepId)}`
  )
}

export async function reorderSteps(
  chainId: string,
  data: ReorderStepsRequest
): Promise<WorkflowChainWithSteps> {
  return apiPost<WorkflowChainWithSteps>(
    `/api/v1/workflow-chains/${encodeURIComponent(chainId)}/steps/reorder`,
    data
  )
}
