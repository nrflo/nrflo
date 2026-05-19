import { useQuery } from '@tanstack/react-query'
import type { UseQueryResult } from '@tanstack/react-query'
import { getGlobalSettings, settingsKeys } from '@/api/settings'
import type { GlobalSettings } from '@/api/settings'

export function useGlobalSettings(): UseQueryResult<GlobalSettings> {
  return useQuery({
    queryKey: settingsKeys.global(),
    queryFn: getGlobalSettings,
  })
}

export function useAPIModeEnabled(): boolean {
  return useGlobalSettings().data?.api_mode_enabled ?? false
}

export function useExperimentalEnabled(): boolean {
  return useGlobalSettings().data?.experimental ?? false
}

export function useExperimentalObserverEnabled(): boolean {
  return useGlobalSettings().data?.experimental_observer_enabled ?? false
}

export interface MenuVisibility {
  newTicket: boolean
  importSpec: boolean
  git: boolean
  chainExecutions: boolean
  schedules: boolean
  workflowChains: boolean
  pythonScripts: boolean
  documentation: boolean
  errors: boolean
  agentSessions: boolean
}

export function useMenuVisibility(): MenuVisibility {
  const { data } = useGlobalSettings()
  return {
    newTicket: data?.menu_new_ticket ?? false,
    importSpec: data?.menu_import_spec ?? false,
    git: data?.menu_git ?? true,
    chainExecutions: data?.menu_chain_executions ?? true,
    schedules: data?.menu_schedules ?? false,
    workflowChains: data?.menu_workflow_chains ?? false,
    pythonScripts: data?.menu_python_scripts ?? false,
    documentation: data?.menu_documentation ?? true,
    errors: data?.menu_errors ?? false,
    agentSessions: data?.menu_agent_sessions ?? false,
  }
}
