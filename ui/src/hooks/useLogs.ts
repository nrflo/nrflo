import { useQuery } from '@tanstack/react-query'
import { getLogs } from '@/api/logs'

export const logsKeys = {
  all: ['logs'] as const,
  byType: (type: 'be', filter?: string) => [...logsKeys.all, type, filter] as const,
}

export function useLogs(type: 'be', filter?: string) {
  return useQuery({
    queryKey: logsKeys.byType(type, filter),
    queryFn: () => getLogs(type, filter),
    refetchInterval: 5000,
  })
}
