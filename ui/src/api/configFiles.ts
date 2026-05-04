import { apiGet, apiPost, apiFetch } from './client'
import type { ConfigFileMeta, ConfigFile, ConfigVersion } from '@/types/config_file'

function encodePathSegments(path: string): string {
  return path.split('/').map(encodeURIComponent).join('/')
}

export async function listConfigFiles(): Promise<ConfigFileMeta[]> {
  return apiGet<ConfigFileMeta[]>('/api/v1/config-files')
}

export async function getConfigFile(path: string): Promise<ConfigFile> {
  return apiGet<ConfigFile>(`/api/v1/config-files/content/${encodePathSegments(path)}`)
}

export async function putConfigFile(path: string, content: string): Promise<ConfigFile> {
  return apiFetch<ConfigFile>(
    `/api/v1/config-files/content/${encodePathSegments(path)}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'text/plain' },
      body: content,
    }
  )
}

export async function getConfigHistory(path: string): Promise<ConfigVersion[]> {
  return apiGet<ConfigVersion[]>(`/api/v1/config-files/history/${encodePathSegments(path)}`)
}

export async function rollbackConfig(path: string, version: number): Promise<ConfigFile> {
  return apiPost<ConfigFile>(
    `/api/v1/config-files/rollback/${encodePathSegments(path)}`,
    { version }
  )
}
