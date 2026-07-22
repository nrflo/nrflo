import { apiDelete, apiGet, apiPatch, apiPost } from './client'

export type ModelProvider = 'anthropic' | 'openai' | 'openrouter'
export type ModelMode = 'cli' | 'api'

export interface Model {
  id: string
  provider: string
  display_name: string
  cli_model: string
  api_model: string
  cli_efforts: string[]
  api_efforts: string[]
  cli_context: number
  api_context: number
  fallback_models: string
  default_effort: string
  read_only: boolean
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface CreateModelRequest {
  id: string
  provider: string
  display_name: string
  cli_model: string
  api_model: string
  cli_efforts: string[]
  api_efforts: string[]
  cli_context: number
  api_context: number
  fallback_models: string
  default_effort: string
}

export type UpdateModelRequest = Partial<Omit<CreateModelRequest, 'id' | 'provider'>> & {
  enabled?: boolean
}

export interface TestModelResult {
  success: boolean
  error?: string
  duration_ms: number
}

export const listModels = () => apiGet<Model[]>('/api/v1/models')
export const getModel = (id: string) => apiGet<Model>(`/api/v1/models/${encodeURIComponent(id)}`)
export const createModel = (req: CreateModelRequest) => apiPost<Model>('/api/v1/models', req)
export const updateModel = (id: string, req: UpdateModelRequest) =>
  apiPatch<Model>(`/api/v1/models/${encodeURIComponent(id)}`, req)
export const deleteModel = (id: string) =>
  apiDelete<{ status: string }>(`/api/v1/models/${encodeURIComponent(id)}`)
export const testModel = (id: string, signal?: AbortSignal) =>
  apiPost<TestModelResult>(`/api/v1/models/${encodeURIComponent(id)}/test`, {}, { signal })
