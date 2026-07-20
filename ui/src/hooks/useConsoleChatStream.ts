import { useEffect, useMemo, useReducer } from 'react'
import { useWebSocketContext } from '@/providers/WebSocketProvider'
import type { WSEvent } from '@/hooks/useWebSocket'
import { useConsoleChat, useConsoleChatMessages } from './useConsoleChats'
import {
  initialSessionStreamState,
  sessionEventReducer,
  mergeStream,
} from '@/components/console/chatStream'
import type { PendingApproval } from '@/types/consoleChat'

// React glue over the pure chatStream reducer: subscribes the session
// channel for sid, feeds every session-scoped event into the reducer, and
// merges the result with persisted history. Re-subscription on reconnect is
// handled inside useWebSocket's onopen — this hook must not re-send on its
// own reconnect-detection, so it only subscribes/unsubscribes on sid change.
export function useConsoleChatStream(sid: string | undefined) {
  const { subscribeSession, unsubscribeSession, addEventListener, removeEventListener } = useWebSocketContext()
  const [stream, dispatch] = useReducer(sessionEventReducer, undefined, initialSessionStreamState)

  const historyQuery = useConsoleChatMessages(sid)
  const detailQuery = useConsoleChat(sid)

  useEffect(() => {
    if (!sid) return
    subscribeSession(sid)
    const handler = (event: WSEvent) => {
      if (event.session_id !== sid) return
      dispatch(event)
    }
    addEventListener(handler)
    return () => {
      removeEventListener(handler)
      unsubscribeSession(sid)
    }
  }, [sid, subscribeSession, unsubscribeSession, addEventListener, removeEventListener])

  const transcript = useMemo(
    () => mergeStream(historyQuery.data?.messages ?? [], stream.deltas),
    [historyQuery.data, stream.deltas]
  )

  // A reload restores turn/pending_approvals from GET /console/chats/{sid}
  // (ConsoleChatDetail.live=false omits them) — live pushes take over once any
  // arrive, so a stale detail snapshot never overrides a fresher live state.
  const turn = stream.turnLive ? stream.turn : (detailQuery.data?.turn ?? stream.turn)

  // Seed with the still-pending approvals from reload; live requests/resolves
  // take over from there (approval rows are never dropped on resolve — see
  // chatStream.ts — so a resolved card keeps its command/cwd for display).
  const approvals = useMemo<PendingApproval[]>(() => {
    const seed = detailQuery.data?.pending_approvals ?? []
    const liveIds = new Set(stream.approvals.map((a) => a.approval_id))
    return [...seed.filter((a) => !liveIds.has(a.approval_id)), ...stream.approvals]
  }, [detailQuery.data, stream.approvals])

  const contextLeft = stream.contextLeft ?? detailQuery.data?.context_left

  const cost = stream.cost ?? detailQuery.data?.cost_estimate

  // Detail seeds the session-approved tool list on reload; the live push
  // (always the full list) takes over once any arrives.
  const sessionApprovals = stream.sessionApprovals ?? detailQuery.data?.session_approvals ?? []

  return {
    transcript,
    turn,
    approvals,
    resolvedApprovals: stream.resolvedApprovals,
    sessionApprovals,
    thinking: stream.thinking,
    errors: stream.errors,
    siblingOpened: stream.siblingOpened,
    rotations: stream.rotations,
    contextLeft,
    cost,
    workDir: detailQuery.data?.work_dir,
    isLoadingHistory: historyQuery.isLoading,
  }
}
