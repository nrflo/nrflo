import { apiGet, apiPatch } from './client'

export interface GlobalSettings {
  low_consumption_mode: boolean
  session_retention_limit: number
  stall_start_timeout_sec: number | null
  stall_running_timeout_sec: number | null
}

export const settingsKeys = {
  all: ['settings'] as const,
  global: () => [...settingsKeys.all, 'global'] as const,
}

export async function getGlobalSettings(): Promise<GlobalSettings> {
  return apiGet<GlobalSettings>('/api/v1/settings')
}

export async function updateGlobalSettings(data: Partial<GlobalSettings>): Promise<void> {
  await apiPatch<{ status: string }>('/api/v1/settings', data)
}
