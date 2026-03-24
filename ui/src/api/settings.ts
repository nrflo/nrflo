import { apiGet, apiPatch } from './client'

export interface GlobalSettings {
  low_consumption_mode: boolean
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
