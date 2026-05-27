import { useQuery } from '@tanstack/react-query'
import { listAvailableTools } from '@/api/availableTools'

export const availableToolKeys = {
  all: ['available-tools'] as const,
  list: () => [...availableToolKeys.all, 'list'] as const,
}

/** Tools an agent can be granted (builtins + project python tools). */
export function useAvailableTools() {
  return useQuery({
    queryKey: availableToolKeys.list(),
    queryFn: listAvailableTools,
  })
}
