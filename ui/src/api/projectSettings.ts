import { apiGet, apiPut } from './client'

export interface ObserverSettings {
  system_context: string
  provider: string
  model: string
}

export interface ArtifactStorageConfig {
  mode: string
  account_id?: string
  bucket?: string
  prefix?: string
  access_key_ref?: string
  secret_key_ref?: string
}

export interface CleanupSettings {
  enabled: boolean
  retention_limit: number
}

export async function getArtifactStorage(projectId: string): Promise<ArtifactStorageConfig> {
  return apiGet<ArtifactStorageConfig>(`/api/v1/projects/${encodeURIComponent(projectId)}/settings/artifact-storage`)
}

export async function setArtifactStorage(projectId: string, cfg: ArtifactStorageConfig): Promise<ArtifactStorageConfig> {
  return apiPut<ArtifactStorageConfig>(`/api/v1/projects/${encodeURIComponent(projectId)}/settings/artifact-storage`, cfg)
}

export async function getCleanup(projectId: string): Promise<CleanupSettings> {
  return apiGet<CleanupSettings>(`/api/v1/projects/${encodeURIComponent(projectId)}/settings/cleanup`)
}

export async function setCleanup(projectId: string, cfg: CleanupSettings): Promise<CleanupSettings> {
  return apiPut<CleanupSettings>(`/api/v1/projects/${encodeURIComponent(projectId)}/settings/cleanup`, cfg)
}

export async function getObserver(projectId: string): Promise<ObserverSettings> {
  return apiGet<ObserverSettings>(`/api/v1/projects/${encodeURIComponent(projectId)}/settings/observer`)
}

export async function setObserver(projectId: string, cfg: Partial<ObserverSettings>): Promise<ObserverSettings> {
  return apiPut<ObserverSettings>(`/api/v1/projects/${encodeURIComponent(projectId)}/settings/observer`, cfg)
}
