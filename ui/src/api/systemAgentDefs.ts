import { apiGet, apiPost, apiPatch, apiDelete } from './client'

export interface SystemAgentDef {
  id: string
  model: string
  timeout: number
  prompt: string
  restart_threshold: number | null
  max_fail_restarts: number | null
  stall_start_timeout_sec: number | null
  stall_running_timeout_sec: number | null
  created_at: string
  updated_at: string
}

export interface CreateSystemAgentDefRequest {
  id: string
  model?: string
  timeout?: number
  prompt: string
  restart_threshold?: number | null
  max_fail_restarts?: number | null
  stall_start_timeout_sec?: number | null
  stall_running_timeout_sec?: number | null
}

export interface UpdateSystemAgentDefRequest {
  model?: string
  timeout?: number
  prompt?: string
  restart_threshold?: number | null
  max_fail_restarts?: number | null
  stall_start_timeout_sec?: number | null
  stall_running_timeout_sec?: number | null
}

export async function listSystemAgentDefs(): Promise<SystemAgentDef[]> {
  return apiGet<SystemAgentDef[]>('/api/v1/system-agents')
}

export async function getSystemAgentDef(id: string): Promise<SystemAgentDef> {
  return apiGet<SystemAgentDef>(`/api/v1/system-agents/${encodeURIComponent(id)}`)
}

export async function createSystemAgentDef(req: CreateSystemAgentDefRequest): Promise<SystemAgentDef> {
  return apiPost<SystemAgentDef>('/api/v1/system-agents', req)
}

export async function updateSystemAgentDef(id: string, req: UpdateSystemAgentDefRequest): Promise<{ status: string }> {
  return apiPatch<{ status: string }>(`/api/v1/system-agents/${encodeURIComponent(id)}`, req)
}

export async function deleteSystemAgentDef(id: string): Promise<{ status: string }> {
  return apiDelete<{ status: string }>(`/api/v1/system-agents/${encodeURIComponent(id)}`)
}
