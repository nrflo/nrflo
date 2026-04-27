import { apiGet, apiPost, apiPut, apiDelete } from './client'
import type { APICredential, APICredentialCreateRequest, APICredentialUpdateRequest } from '@/types/apiCredential'

export async function listAPICredentials(project_id?: string): Promise<APICredential[]> {
  const url = project_id
    ? `/api/v1/api-credentials?project_id=${encodeURIComponent(project_id)}`
    : '/api/v1/api-credentials'
  return apiGet<APICredential[]>(url)
}

export async function getAPICredential(id: string): Promise<APICredential> {
  return apiGet<APICredential>(`/api/v1/api-credentials/${encodeURIComponent(id)}`)
}

export async function createAPICredential(req: APICredentialCreateRequest): Promise<APICredential> {
  return apiPost<APICredential>('/api/v1/api-credentials', req)
}

export async function updateAPICredential(id: string, req: APICredentialUpdateRequest): Promise<APICredential> {
  return apiPut<APICredential>(`/api/v1/api-credentials/${encodeURIComponent(id)}`, req)
}

export async function deleteAPICredential(id: string): Promise<{ status: string }> {
  return apiDelete<{ status: string }>(`/api/v1/api-credentials/${encodeURIComponent(id)}`)
}
