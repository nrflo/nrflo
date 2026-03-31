import { useState, useEffect, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, Terminal, CheckCircle, XCircle, Clock, AlertTriangle, Timer, Loader2 } from 'lucide-react'
import { cn, formatDateTime, formatElapsedTime, contextLeftColor } from '@/lib/utils'
import { Badge } from '@/components/ui/Badge'
import { getSessionMessages } from '@/api/tickets'
import type { AgentSession, AgentSessionStatus } from '@/types/workflow'

export function statusColor(status: AgentSessionStatus): string {
  switch (status) {
    case 'running':
      return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200'
    case 'completed':
      return 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200'
    case 'failed':
      return 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200'
    case 'timeout':
      return 'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200'
    default:
      return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200'
  }
}

export function StatusIcon({ status }: { status: AgentSessionStatus }) {
  switch (status) {
    case 'running':
      return <Clock className="h-3 w-3 animate-pulse" />
    case 'completed':
      return <CheckCircle className="h-3 w-3" />
    case 'failed':
      return <XCircle className="h-3 w-3" />
    case 'timeout':
      return <AlertTriangle className="h-3 w-3" />
    default:
      return null
  }
}

interface AgentSessionCardProps {
  session: AgentSession
  defaultExpanded?: boolean
  children?: ReactNode
}

export function AgentSessionCard({ session, defaultExpanded = false, children }: AgentSessionCardProps) {
  const [expanded, setExpanded] = useState(defaultExpanded)
  const msgCount = session.message_count || 0
  const hasMessages = msgCount > 0
  const isRunning = session.status === 'running'

  // Lazy-load messages from API when expanded (updates via WS messages.updated events)
  const { data: messagesData, isLoading: messagesLoading } = useQuery({
    queryKey: ['session-messages', session.id],
    queryFn: () => getSessionMessages(session.id),
    enabled: expanded && hasMessages,
    staleTime: isRunning ? 2000 : 30000,
  })

  const messages = messagesData?.messages ?? []

  // Update elapsed time every second for running sessions
  const [, setTick] = useState(0)
  useEffect(() => {
    if (!isRunning) return
    const interval = setInterval(() => setTick(t => t + 1), 1000)
    return () => clearInterval(interval)
  }, [isRunning])

  // Calculate elapsed time
  const elapsedTime = isRunning
    ? formatElapsedTime(session.started_at || session.created_at)
    : formatElapsedTime(session.started_at || session.created_at, session.ended_at || session.updated_at)

  return (
    <div className="border border-border rounded-lg overflow-hidden">
      <button
        onClick={() => hasMessages && setExpanded(!expanded)}
        disabled={!hasMessages}
        className={cn(
          'w-full flex items-center gap-3 p-2 text-left transition-colors',
          hasMessages && 'hover:bg-muted/50 cursor-pointer',
          !hasMessages && 'cursor-default',
          session.status === 'running' && 'bg-yellow-50/50 dark:bg-yellow-900/10'
        )}
      >
        {hasMessages && (
          <span className="text-muted-foreground">
            {expanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
          </span>
        )}
        <Terminal className={cn(
          'h-4 w-4 shrink-0',
          session.status === 'running' && 'text-yellow-600 dark:text-yellow-400 animate-pulse',
          session.status === 'completed' && 'text-green-600 dark:text-green-400',
          session.status === 'failed' && 'text-red-600 dark:text-red-400',
          session.status === 'timeout' && 'text-orange-600 dark:text-orange-400'
        )} />
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="font-mono text-sm">{session.agent_type}</span>
            {session.model_id && (
              <Badge variant="outline" className="text-xs">
                {session.model_id}
              </Badge>
            )}
          </div>
          {children}
        </div>
        <div className="flex items-center gap-2">
          <span className="flex items-center gap-1 text-xs text-muted-foreground">
            <Timer className="h-3 w-3" />
            {elapsedTime}
          </span>
          {session.context_left != null && (
            <span className={cn(
              'text-xs font-mono px-1.5 py-0.5',
              contextLeftColor(session.context_left)
            )}>
              {session.context_left}% ctx
            </span>
          )}
          {hasMessages && (
            <span className="text-xs text-muted-foreground">
              {msgCount} msg{msgCount !== 1 ? 's' : ''}
            </span>
          )}
          <Badge className={cn('text-xs flex items-center gap-1', statusColor(session.status))}>
            <StatusIcon status={session.status} />
            {session.status}
          </Badge>
        </div>
      </button>

      {expanded && hasMessages && (
        <div className="p-3 border-t border-border bg-muted/20 space-y-2">
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span>
              {messagesData ? `${messagesData.total} total messages` : `${msgCount} messages`}
            </span>
            <span>Updated: {formatDateTime(session.updated_at)}</span>
          </div>
          {messagesLoading ? (
            <div className="flex items-center justify-center py-4">
              <Loader2 className="h-4 w-4 spin-sync text-muted-foreground" />
              <span className="ml-2 text-xs text-muted-foreground">Loading messages...</span>
            </div>
          ) : (
            <div className="space-y-1 font-mono text-xs">
              {[...messages].reverse().map((msg, idx) => (
                <div
                  key={idx}
                  className="p-2 bg-background rounded border border-border/50 whitespace-pre-wrap break-words"
                >
                  {msg.content}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
