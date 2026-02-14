import { apiGet, apiPost, apiPatch } from './client'
import type {
  ChainExecution,
  ChainCreateRequest,
  ChainUpdateRequest,
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

export async function runEpicWorkflow(
  ticketId: string,
  params: RunEpicWorkflowParams
): Promise<ChainExecution> {
  return apiPost<ChainExecution>(
    `/api/v1/tickets/${encodeURIComponent(ticketId)}/workflow/run-epic`,
    params
  )
}
