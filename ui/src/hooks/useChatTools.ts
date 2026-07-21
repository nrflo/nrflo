import { useMutation, useQuery } from '@tanstack/react-query'
import { getConsoleChatTools, invokeConsoleChatTool } from '@/api/consoleChats'
import { consoleChatKeys } from '@/hooks/useConsoleChats'
import type { ConsoleChatInvokeRequest } from '@/types/consoleChat'

// GET /console/chats/{sid}/tools backs the composer's '/invoke' directive
// (nrworkflow-362119). Fetched once per chat — no polling, WS-only realtime
// invariant — mirroring useProjectSkills.
export function useChatTools(sid: string) {
  return useQuery({
    queryKey: consoleChatKeys.tools(sid),
    queryFn: () => getConsoleChatTools(sid),
    enabled: !!sid,
    staleTime: Infinity,
  })
}

// No onSuccess invalidation: the persisted user + tool transcript rows
// arrive via the existing messages.updated WS event, not this response.
export function useInvokeChatTool() {
  return useMutation({
    mutationFn: ({ sid, ...body }: { sid: string } & ConsoleChatInvokeRequest) => invokeConsoleChatTool(sid, body),
  })
}
