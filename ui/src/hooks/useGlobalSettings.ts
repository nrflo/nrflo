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
