import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, MessageSquare, FileText, Loader2, CheckCircle, XCircle, Cpu, Timer } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { LogMessage } from './LogMessage'
import { cn } from '@/lib/utils'
import { getSessionMessages, getSessionRawOutput } from '@/api/tickets'
import type { SelectedAgentData } from './PhaseGraph/types'

function formatDuration(durationSec?: number): string {
  if (!durationSec) return '0s'
  const mins = Math.floor(durationSec / 60)
  const secs = durationSec % 60
  if (mins > 0) return secs > 0 ? `${mins}m ${secs}s` : `${mins}m`
  return `${secs}s`
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

interface AgentLogDetailProps {
  selectedAgent: SelectedAgentData
  onBack: () => void
}

export function AgentLogDetail({ selectedAgent, onBack }: AgentLogDetailProps) {
  const [showRawOutput, setShowRawOutput] = useState(false)
  const messagesStartRef = useRef<HTMLDivElement>(null)

  const { agent, historyEntry, session, phaseName } = selectedAgent
  const isRunning = agent && !agent.result
  const result = agent?.result || historyEntry?.result
  const modelId = agent?.model_id || historyEntry?.model_id
  const modelName = modelId
    ? modelId.split('-').slice(-2).join('-') || modelId
    : agent?.cli || historyEntry?.agent_type || 'agent'
  const duration = historyEntry?.duration_sec ? formatDuration(historyEntry.duration_sec) : null

  // Lazy-load messages from API
  const { data: messagesData, isLoading: messagesLoading } = useQuery({
    queryKey: ['session-messages', session?.id],
    queryFn: () => getSessionMessages(session!.id),
    enabled: !!session?.id && !showRawOutput,
    staleTime: isRunning ? 2000 : 30000,
    refetchInterval: isRunning && !showRawOutput ? 3000 : false,
  })

  // Lazy-load raw output only when toggled
  const { data: rawOutputData, isLoading: rawOutputLoading } = useQuery({
    queryKey: ['session-raw-output', session?.id],
    queryFn: () => getSessionRawOutput(session!.id),
    enabled: !!session?.id && showRawOutput,
    staleTime: isRunning ? 2000 : 30000,
    refetchInterval: isRunning && showRawOutput ? 3000 : false,
  })

  const messages = messagesData?.messages ?? []

  // Reset raw output view when agent changes
  useEffect(() => {
    setShowRawOutput(false)
  }, [session?.id])

  // Auto-scroll to top when new messages arrive
  useEffect(() => {
    if (messagesStartRef.current) {
      messagesStartRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [messages.length])

  const hasRawOutput = (session?.raw_output_size ?? 0) > 0 || isRunning

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

      {/* Messages / Raw Output toggle */}
      {hasRawOutput && (
        <div className="flex items-center gap-2 px-3 py-2 border-b border-border shrink-0">
          <button
            onClick={() => setShowRawOutput(false)}
            className={cn(
              'flex items-center gap-1.5 px-2.5 py-1 text-xs rounded-md transition-colors',
              !showRawOutput
                ? 'bg-accent text-accent-foreground font-medium'
                : 'text-muted-foreground hover:text-foreground hover:bg-accent/50'
            )}
          >
            <MessageSquare className="h-3 w-3" />
            Messages
          </button>
          <button
            onClick={() => setShowRawOutput(true)}
            className={cn(
              'flex items-center gap-1.5 px-2.5 py-1 text-xs rounded-md transition-colors',
              showRawOutput
                ? 'bg-accent text-accent-foreground font-medium'
                : 'text-muted-foreground hover:text-foreground hover:bg-accent/50'
            )}
          >
            <FileText className="h-3 w-3" />
            Raw
            {session?.raw_output_size ? (
              <span className="opacity-70">({formatBytes(session.raw_output_size)})</span>
            ) : null}
          </button>
        </div>
      )}

      {/* Content area */}
      <div className="flex-1 overflow-y-auto px-3 py-2">
        {showRawOutput ? (
          rawOutputLoading ? (
            <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
              <Loader2 className="h-6 w-6 mb-2 animate-spin opacity-50" />
              <p className="text-xs">Loading raw output...</p>
            </div>
          ) : rawOutputData?.raw_output ? (
            <pre className="text-xs font-mono whitespace-pre-wrap break-all bg-muted/50 rounded-lg p-3 overflow-auto">
              {rawOutputData.raw_output}
            </pre>
          ) : (
            <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
              <FileText className="h-8 w-8 mb-2 opacity-30" />
              <p className="text-xs">No raw output available</p>
            </div>
          )
        ) : messagesLoading ? (
          <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
            <Loader2 className="h-6 w-6 mb-2 animate-spin opacity-50" />
            <p className="text-xs">Loading messages...</p>
          </div>
        ) : messages.length > 0 ? (
          <div className="space-y-2">
            <div ref={messagesStartRef} />
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-2">
              <MessageSquare className="h-3 w-3" />
              <span>{messagesData ? `${messagesData.total} messages` : `${messages.length} messages`}</span>
            </div>
            {[...messages].reverse().map((msg, i, arr) => {
              const nextMsg = arr[i + 1]
              return (
                <LogMessage
                  key={i}
                  message={msg.content}
                  variant="full"
                  timestamp={msg.created_at || undefined}
                  nextTimestamp={nextMsg?.created_at || undefined}
                />
              )
            })}
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
