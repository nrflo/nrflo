import { apiDelete, apiGet, apiPatch, apiPost } from './client'

export type APIWire = 'responses' | 'chat_completions'

export interface CustomProvider {
  name: string
  base_url: string
  api_key: string
  api_wire: APIWire
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface CreateCustomProviderRequest {
  name: string
  base_url: string
  api_key: string
  api_wire: APIWire
}

export interface UpdateCustomProviderRequest {
  base_url?: string
  api_key?: string
  api_wire?: APIWire
  enabled?: boolean
}

export interface CheckConnectionRequest {
  base_url: string
  api_key: string
  api_wire: APIWire
}

export interface CheckConnectionResult {
  ok: boolean
  models: string[]
  error?: string
}

export const listCustomProviders = () => apiGet<CustomProvider[]>('/api/v1/custom-providers')
export const getCustomProvider = (name: string) =>
  apiGet<CustomProvider>(`/api/v1/custom-providers/${encodeURIComponent(name)}`)
export const createCustomProvider = (req: CreateCustomProviderRequest) =>
  apiPost<CustomProvider>('/api/v1/custom-providers', req)
export const updateCustomProvider = (name: string, req: UpdateCustomProviderRequest) =>
  apiPatch<CustomProvider>(`/api/v1/custom-providers/${encodeURIComponent(name)}`, req)
export const deleteCustomProvider = (name: string) =>
  apiDelete<{ status: string }>(`/api/v1/custom-providers/${encodeURIComponent(name)}`)
export const checkCustomProviderConnection = (req: CheckConnectionRequest, signal?: AbortSignal) =>
  apiPost<CheckConnectionResult>('/api/v1/custom-providers/check', req, { signal })
