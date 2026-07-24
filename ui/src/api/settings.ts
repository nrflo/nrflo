import { apiGet, apiPatch } from './client'

export interface GlobalSettings {
  api_mode_enabled: boolean
  api_native_tools_enabled: boolean
  api_via_cli_enabled: boolean
  claude_system_prompt_override_enabled: boolean
  low_consumption_mode: boolean
  simplified_agents_graph: boolean
  experimental: boolean
  capture_thinking_enabled: boolean
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
  dynamic_workflow_auto_enabled: boolean
  console_yolo: boolean
  context_budget_fraction: number | null
  context_budget_default: number | null
  context_decay_turns: number | null
  cache_ttl_sec: number | null
  min_epoch_interval_calls: number | null
  proactive_restart_threshold_default: number | null
  proactive_restart_min_interval_sec: number | null
  proactive_restart_max_per_session: number | null
  proactive_restart_boundary_window_turns: number | null
  proactive_restart_console_pct: number | null
  refinery_fold_start_context_pct: number | null
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
