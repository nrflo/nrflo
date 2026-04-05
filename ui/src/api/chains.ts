import { apiGet, apiPost, apiPatch, apiDelete } from './client'
import type {
  ChainExecution,
  ChainPreviewResponse,
  ChainCreateRequest,
  ChainUpdateRequest,
  ChainAppendRequest,
  ChainRemoveRequest,
} from '@/types/chain'

export interface ListChainsParams {
  status?: string
  epic_ticket_id?: string
}

export interface RunEpicWorkflowParams {
  workflow_name: string
  start: boolean
}

export async function listChains(
  params?: ListChainsParams
): Promise<ChainExecution[]> {
  const searchParams = new URLSearchParams()
  if (params?.status) searchParams.set('status', params.status)
  if (params?.epic_ticket_id) searchParams.set('epic_ticket_id', params.epic_ticket_id)
  const query = searchParams.toString()
  return apiGet<ChainExecution[]>(`/api/v1/chains${query ? `?${query}` : ''}`)
}

export async function getChain(id: string): Promise<ChainExecution> {
  return apiGet<ChainExecution>(`/api/v1/chains/${encodeURIComponent(id)}`)
}

export async function previewChain(
  data: { ticket_ids: string[] }
): Promise<ChainPreviewResponse> {
  return apiPost<ChainPreviewResponse>('/api/v1/chains/preview', data)
}

export async function createChain(
  data: ChainCreateRequest
): Promise<ChainExecution> {
  return apiPost<ChainExecution>('/api/v1/chains', data)
}

export async function updateChain(
  id: string,
  data: ChainUpdateRequest
): Promise<ChainExecution> {
  return apiPatch<ChainExecution>(
    `/api/v1/chains/${encodeURIComponent(id)}`,
    data
  )
}

export async function startChain(
  id: string
): Promise<{ status: string; chain_id: string }> {
  return apiPost<{ status: string; chain_id: string }>(
    `/api/v1/chains/${encodeURIComponent(id)}/start`,
    {}
  )
}

export async function cancelChain(
  id: string
): Promise<{ status: string; chain_id: string }> {
  return apiPost<{ status: string; chain_id: string }>(
    `/api/v1/chains/${encodeURIComponent(id)}/cancel`,
    {}
  )
}

export async function appendToChain(
  id: string,
  data: ChainAppendRequest
): Promise<ChainExecution> {
  return apiPost<ChainExecution>(
    `/api/v1/chains/${encodeURIComponent(id)}/append`,
    data
  )
}

export async function deleteChain(id: string): Promise<void> {
  await apiDelete(`/api/v1/chains/${encodeURIComponent(id)}`)
}

export async function removeFromChain(
  id: string,
  data: ChainRemoveRequest
): Promise<ChainExecution> {
  return apiPost<ChainExecution>(
    `/api/v1/chains/${encodeURIComponent(id)}/remove-items`,
    data
  )
}

export async function runEpicWorkflow(
  ticketId: string,
  params: RunEpicWorkflowParams
): Promise<ChainExecution> {
  return apiPost<ChainExecution>(
    `/api/v1/tickets/${encodeURIComponent(ticketId)}/workflow/run-epic`,
    params
  )
}
