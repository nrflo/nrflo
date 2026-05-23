import { apiGet, apiPost, apiPatch, apiDelete } from './client'

export type APIProviderName = 'anthropic' | 'openai'

export interface APIModel {
  id: string
  provider: APIProviderName
  display_name: string
  mapped_model: string
  reasoning_effort: string
  context_length: number
  read_only: boolean
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface CreateAPIModelRequest {
  id: string
  provider: APIProviderName
  display_name: string
  mapped_model: string
  reasoning_effort?: string
  context_length?: number
}

export interface UpdateAPIModelRequest {
  display_name?: string
  mapped_model?: string
  reasoning_effort?: string
  context_length?: number
  enabled?: boolean
}

export async function listAPIModels(): Promise<APIModel[]> {
  return apiGet<APIModel[]>('/api/v1/api-models')
}

export async function getAPIModel(id: string): Promise<APIModel> {
  return apiGet<APIModel>(`/api/v1/api-models/${encodeURIComponent(id)}`)
}

export async function createAPIModel(req: CreateAPIModelRequest): Promise<APIModel> {
  return apiPost<APIModel>('/api/v1/api-models', req)
}

export async function updateAPIModel(id: string, req: UpdateAPIModelRequest): Promise<{ status: string }> {
  return apiPatch<{ status: string }>(`/api/v1/api-models/${encodeURIComponent(id)}`, req)
}

export async function deleteAPIModel(id: string): Promise<{ status: string }> {
  return apiDelete<{ status: string }>(`/api/v1/api-models/${encodeURIComponent(id)}`)
}
