import { apiGet, apiPatch } from './client'

export interface GlobalSettings {
  api_mode_enabled: boolean
  low_consumption_mode: boolean
  context_save_via_agent: boolean
  simplified_agents_graph: boolean
  experimental: boolean
  experimental_observer_enabled: boolean
  observer_system_context: string
  observer_provider: string
  observer_model: string
  stall_start_timeout_sec: number | null
  stall_running_timeout_sec: number | null
  menu_new_ticket: boolean
  menu_import_spec: boolean
  menu_git: boolean
  menu_chain_executions: boolean
  menu_schedules: boolean
  menu_workflow_chains: boolean
  menu_python_scripts: boolean
  menu_documentation: boolean
  menu_errors: boolean
  menu_agent_sessions: boolean
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
