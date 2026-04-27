import { apiGet, apiPost, apiPut, apiDelete } from './client'
import type { ToolDefinition, ToolDefinitionCreateRequest, ToolDefinitionUpdateRequest } from '@/types/toolDefinition'

export interface ListToolDefinitionsFilters {
  project_id?: string
  workflow_id?: string
}

export async function listToolDefinitions(filters?: ListToolDefinitionsFilters): Promise<ToolDefinition[]> {
  const params = new URLSearchParams()
  if (filters?.project_id) params.set('project_id', filters.project_id)
  if (filters?.workflow_id) params.set('workflow_id', filters.workflow_id)
  const qs = params.toString()
  return apiGet<ToolDefinition[]>(`/api/v1/tool-definitions${qs ? `?${qs}` : ''}`)
}

export async function getToolDefinition(id: string): Promise<ToolDefinition> {
  return apiGet<ToolDefinition>(`/api/v1/tool-definitions/${encodeURIComponent(id)}`)
}

export async function createToolDefinition(req: ToolDefinitionCreateRequest): Promise<ToolDefinition> {
  return apiPost<ToolDefinition>('/api/v1/tool-definitions', req)
}

export async function updateToolDefinition(id: string, req: ToolDefinitionUpdateRequest): Promise<ToolDefinition> {
  return apiPut<ToolDefinition>(`/api/v1/tool-definitions/${encodeURIComponent(id)}`, req)
}

export async function deleteToolDefinition(id: string): Promise<{ status: string }> {
  return apiDelete<{ status: string }>(`/api/v1/tool-definitions/${encodeURIComponent(id)}`)
}
