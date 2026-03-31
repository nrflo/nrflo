import { useQuery } from '@tanstack/react-query'
import { fetchSessionPrompt } from '@/api/agents'

export function useSessionPrompt(sessionId: string | undefined, enabled: boolean) {
  return useQuery({
    queryKey: ['session-prompt', sessionId],
    queryFn: () => fetchSessionPrompt(sessionId!),
    enabled: !!sessionId && enabled,
    staleTime: Infinity,
  })
}
