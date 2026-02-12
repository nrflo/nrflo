import { useEffect, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, MessageSquare, Loader2, CheckCircle, XCircle, Cpu, Timer } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { parseToolName, ToolBadge } from './LogMessage'
import { cn } from '@/lib/utils'
import { getSessionMessages } from '@/api/tickets'
import type { SelectedAgentData } from './PhaseGraph/types'

function formatDuration(durationSec?: number): string {
  if (!durationSec) return '0s'
  const mins = Math.floor(durationSec / 60)
  const secs = durationSec % 60
  if (mins > 0) return secs > 0 ? `${mins}m ${secs}s` : `${mins}m`
  return `${secs}s`
}

function formatTime(dateStr: string): string {
  if (!dateStr) return ''
  const d = new Date(dateStr)
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

interface AgentLogDetailProps {
  selectedAgent: SelectedAgentData
  onBack: () => void
}

export function AgentLogDetail({ selectedAgent, onBack }: AgentLogDetailProps) {
  const messagesStartRef = useRef<HTMLDivElement>(null)

  const { agent, historyEntry, session, phaseName } = selectedAgent
  const isRunning = agent && !agent.result
  const result = agent?.result || historyEntry?.result
  const modelId = agent?.model_id || historyEntry?.model_id
  const modelName = modelId
    ? modelId.split('-').slice(-2).join('-') || modelId
    : agent?.cli || historyEntry?.agent_type || 'agent'
  const duration = historyEntry?.duration_sec ? formatDuration(historyEntry.duration_sec) : null

  const { data: messagesData, isLoading: messagesLoading } = useQuery({
    queryKey: ['session-messages', session?.id],
    queryFn: () => getSessionMessages(session!.id),
    enabled: !!session?.id,
    staleTime: isRunning ? 2000 : 30000,
    refetchInterval: isRunning ? 3000 : false,
  })

  const messages = messagesData?.messages ?? []

  // Auto-scroll to top when new messages arrive
  useEffect(() => {
    if (messagesStartRef.current) {
      messagesStartRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [messages.length])

  return (
    <div className="flex flex-col h-full">
      {/* Header with back button and agent info */}
      <div className="flex items-center gap-2 px-3 py-2 border-b border-border shrink-0">
        <button
          onClick={onBack}
          className="flex items-center justify-center w-6 h-6 rounded hover:bg-muted transition-colors shrink-0"
        >
          <ArrowLeft className="h-3.5 w-3.5" />
        </button>
        <div className={cn(
          'flex items-center justify-center w-7 h-7 rounded-full shrink-0',
          isRunning && 'bg-yellow-100 dark:bg-yellow-900/30',
          result === 'pass' && 'bg-green-100 dark:bg-green-900/30',
          result === 'fail' && 'bg-red-100 dark:bg-red-900/30',
          !result && !isRunning && 'bg-gray-100 dark:bg-gray-800'
        )}>
          {isRunning && <Loader2 className="h-3.5 w-3.5 text-yellow-600 dark:text-yellow-400 animate-spin" />}
          {result === 'pass' && <CheckCircle className="h-3.5 w-3.5 text-green-600 dark:text-green-400" />}
          {result === 'fail' && <XCircle className="h-3.5 w-3.5 text-red-600 dark:text-red-400" />}
          {!result && !isRunning && <Cpu className="h-3.5 w-3.5 text-muted-foreground" />}
        </div>
        <div className="min-w-0 flex-1">
          <div className="text-sm font-medium truncate">{phaseName.replace(/_/g, ' ')}</div>
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span>{modelName}</span>
            {duration && (
              <>
                <span>·</span>
                <Timer className="h-3 w-3" />
                <span>{duration}</span>
              </>
            )}
            {result && (
              <>
                <span>·</span>
                <Badge variant={result === 'pass' ? 'success' : 'destructive'} className="text-[10px] px-1 py-0">
                  {result}
                </Badge>
              </>
            )}
          </div>
        </div>
      </div>

      {/* Content area */}
      <div className="flex-1 overflow-y-auto px-3 py-2">
        {messagesLoading ? (
          <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
            <Loader2 className="h-6 w-6 mb-2 animate-spin opacity-50" />
            <p className="text-xs">Loading messages...</p>
          </div>
        ) : messages.length > 0 ? (
          <div>
            <div ref={messagesStartRef} />
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-2">
              <MessageSquare className="h-3 w-3" />
              <span>{messagesData ? `${messagesData.total} messages` : `${messages.length} messages`}</span>
            </div>
            <table className="w-full text-xs font-mono border-collapse">
              <thead>
                <tr className="text-left text-muted-foreground border-b border-border">
                  <th className="py-1 pr-2 font-medium w-[70px]">Time</th>
                  <th className="py-1 pr-2 font-medium w-[70px]">Tool</th>
                  <th className="py-1 font-medium">Message</th>
                </tr>
              </thead>
              <tbody>
                {[...messages].reverse().map((msg, i) => {
                  const { toolName, rest } = parseToolName(msg.content)
                  return (
                    <tr key={i} className="border-b border-border/50 align-top">
                      <td className="py-1 pr-2 text-muted-foreground whitespace-nowrap">
                        {formatTime(msg.created_at)}
                      </td>
                      <td className="py-1 pr-2">
                        {toolName && <ToolBadge name={toolName} />}
                      </td>
                      <td className="py-1 whitespace-pre-wrap break-words text-foreground/90">
                        {rest}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
            <MessageSquare className="h-8 w-8 mb-2 opacity-30" />
            <p className="text-xs">No messages available</p>
          </div>
        )}
      </div>
    </div>
  )
}
