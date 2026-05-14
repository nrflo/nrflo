import { apiGet, apiPatch } from './client'

export type ProviderName = 'claude' | 'codex' | 'opencode'
export type CLIMode = 'cli' | 'cli_interactive'

export interface ProviderSettings {
  [provider: string]: {
    modes: CLIMode[]
  }
}

export async function listProviders(): Promise<ProviderSettings> {
  return apiGet<ProviderSettings>('/api/v1/providers')
}

export async function updateProvider(name: ProviderName, modes: CLIMode[]): Promise<{ status: string }> {
  return apiPatch<{ status: string }>(`/api/v1/providers/${encodeURIComponent(name)}`, { modes })
}
