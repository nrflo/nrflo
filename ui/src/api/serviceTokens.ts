import { apiGet, apiPost, apiDelete } from './client'
import type { ServiceToken, CreateServiceTokenResponse } from '@/types/serviceToken'

export async function listServiceTokens(): Promise<ServiceToken[]> {
  return apiGet<ServiceToken[]>('/api/v1/service-tokens')
}

export async function createServiceToken(
  projectId: string,
  name: string,
  scope: 'project' | 'global' = 'project'
): Promise<CreateServiceTokenResponse> {
  return apiPost<CreateServiceTokenResponse>('/api/v1/service-tokens', {
    project_id: scope === 'global' ? '' : projectId,
    scope,
    name,
  })
}

export async function deleteServiceToken(id: string): Promise<void> {
  return apiDelete<void>(`/api/v1/service-tokens/${encodeURIComponent(id)}`)
}
