import { useQuery } from '@tanstack/react-query'
import { getLogs } from '@/api/logs'

export const logsKeys = {
  all: ['logs'] as const,
  byType: (type: 'be' | 'fe') => [...logsKeys.all, type] as const,
}

export function useLogs(type: 'be' | 'fe') {
  return useQuery({
    queryKey: logsKeys.byType(type),
    queryFn: () => getLogs(type),
    refetchInterval: 5000,
  })
}
