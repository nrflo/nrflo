import { apiGet, apiPut, apiDelete } from './client'

export interface ProjectEnvVar {
  project_id?: string
  name: string
  value: string
  created_at: string
  updated_at: string
}

export async function listEnvVars(projectId: string): Promise<ProjectEnvVar[]> {
  return apiGet<ProjectEnvVar[]>(`/api/v1/projects/${encodeURIComponent(projectId)}/env-vars`)
}

export async function putEnvVar(projectId: string, name: string, value: string): Promise<ProjectEnvVar> {
  return apiPut<ProjectEnvVar>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/env-vars/${encodeURIComponent(name)}`,
    { value }
  )
}

export async function deleteEnvVar(projectId: string, name: string): Promise<void> {
  return apiDelete<void>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/env-vars/${encodeURIComponent(name)}`
  )
}
