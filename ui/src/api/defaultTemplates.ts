import { apiGet, apiPost, apiPatch, apiDelete } from './client'

export interface DefaultTemplate {
  id: string
  name: string
  type: string
  template: string
  readonly: boolean
  default_template?: string
  created_at: string
  updated_at: string
}

export interface CreateDefaultTemplateRequest {
  id: string
  name: string
  template: string
  type?: string
}

export interface UpdateDefaultTemplateRequest {
  name?: string
  template?: string
  type?: string
}

export async function listDefaultTemplates(type?: string): Promise<DefaultTemplate[]> {
  const url = type ? `/api/v1/default-templates?type=${encodeURIComponent(type)}` : '/api/v1/default-templates'
  return apiGet<DefaultTemplate[]>(url)
}

export async function getDefaultTemplate(id: string): Promise<DefaultTemplate> {
  return apiGet<DefaultTemplate>(`/api/v1/default-templates/${encodeURIComponent(id)}`)
}

export async function createDefaultTemplate(req: CreateDefaultTemplateRequest): Promise<DefaultTemplate> {
  return apiPost<DefaultTemplate>('/api/v1/default-templates', req)
}

export async function updateDefaultTemplate(id: string, req: UpdateDefaultTemplateRequest): Promise<{ status: string }> {
  return apiPatch<{ status: string }>(`/api/v1/default-templates/${encodeURIComponent(id)}`, req)
}

export async function deleteDefaultTemplate(id: string): Promise<{ status: string }> {
  return apiDelete<{ status: string }>(`/api/v1/default-templates/${encodeURIComponent(id)}`)
}

export async function restoreDefaultTemplate(id: string): Promise<{ status: string }> {
  return apiPost<{ status: string }>(`/api/v1/default-templates/${encodeURIComponent(id)}/restore`)
}
