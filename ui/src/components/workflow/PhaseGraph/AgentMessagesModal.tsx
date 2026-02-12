import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CheckCircle, XCircle, Timer, Cpu, MessageSquare, Loader2, FileText } from 'lucide-react'
import { Dialog, DialogHeader, DialogBody } from '@/components/ui/Dialog'
import { Badge } from '@/components/ui/Badge'
import { LogMessage } from '@/components/workflow/LogMessage'
import { cn } from '@/lib/utils'
import { getSessionMessages, getSessionRawOutput } from '@/api/tickets'
import type { ActiveAgentV4, AgentSession, AgentHistoryEntry } from '@/types/workflow'

interface AgentMessagesModalProps {
  open: boolean
  onClose: () => void
  phaseName: string
  agent?: ActiveAgentV4
  historyEntry?: AgentHistoryEntry
  session?: AgentSession
}

function formatDuration(durationSec?: number): string {
  if (!durationSec) return '0s'
  const mins = Math.floor(durationSec / 60)
  const secs = durationSec % 60
  if (mins > 0) {
    return secs > 0 ? `${mins}m ${secs}s` : `${mins}m`
  }
  return `${secs}s`
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export function AgentMessagesModal({
  open,
  onClose,
  phaseName,
  agent,
  historyEntry,
  session,
}: AgentMessagesModalProps) {
  const messagesStartRef = useRef<HTMLDivElement>(null)
  const [showRawOutput, setShowRawOutput] = useState(false)
  const isRunning = agent && !agent.result
  const result = agent?.result || historyEntry?.result
  const modelId = agent?.model_id || historyEntry?.model_id
  const modelName = modelId
    ? modelId.split('-').slice(-2).join('-') || modelId
    : agent?.cli || historyEntry?.agent_type || 'agent'
  const duration = historyEntry?.duration_sec ? formatDuration(historyEntry.duration_sec) : null

  // Lazy-load messages from API when modal is open
  const { data: messagesData, isLoading: messagesLoading } = useQuery({
    queryKey: ['session-messages', session?.id],
    queryFn: () => getSessionMessages(session!.id),
    enabled: open && !!session?.id && !showRawOutput,
    staleTime: isRunning ? 2000 : 30000,
    refetchInterval: isRunning && open && !showRawOutput ? 3000 : false,
  })

  // Lazy-load raw output only when toggled
  const { data: rawOutputData, isLoading: rawOutputLoading } = useQuery({
    queryKey: ['session-raw-output', session?.id],
    queryFn: () => getSessionRawOutput(session!.id),
    enabled: open && !!session?.id && showRawOutput,
    staleTime: isRunning ? 2000 : 30000,
    refetchInterval: isRunning && open && showRawOutput ? 3000 : false,
  })

  const messages = messagesData?.messages ?? []
  const messageContents = messages.map(m => m.content)

  // Reset raw output view when modal closes
  useEffect(() => {
    if (!open) setShowRawOutput(false)
  }, [open])

  // Auto-scroll to top when new messages arrive (latest are at top)
  useEffect(() => {
    if (open && messagesStartRef.current) {
      messagesStartRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [open, messageContents.length])

  const hasRawOutput = (session?.raw_output_size ?? 0) > 0 || isRunning

  return (
    <Dialog open={open} onClose={onClose}>
      <DialogHeader onClose={onClose}>
        <div className="flex items-center gap-3">
          {/* Status icon */}
          <div
            className={cn(
              'flex items-center justify-center w-10 h-10 rounded-full',
              isRunning && 'bg-yellow-100 dark:bg-yellow-900/30',
              result === 'pass' && 'bg-green-100 dark:bg-green-900/30',
              result === 'fail' && 'bg-red-100 dark:bg-red-900/30',
              !result && !isRunning && 'bg-gray-100 dark:bg-gray-800'
            )}
          >
            {isRunning && <Loader2 className="h-5 w-5 text-yellow-600 dark:text-yellow-400 animate-spin" />}
            {result === 'pass' && <CheckCircle className="h-5 w-5 text-green-600 dark:text-green-400" />}
            {result === 'fail' && <XCircle className="h-5 w-5 text-red-600 dark:text-red-400" />}
            {!result && !isRunning && <Cpu className="h-5 w-5 text-muted-foreground" />}
          </div>

          <div>
            <h2 className="text-lg font-semibold">
              {phaseName.replace(/_/g, ' ')}
            </h2>
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <span>{modelName}</span>
              {duration && (
                <>
                  <span>·</span>
                  <span className="flex items-center gap-1">
                    <Timer className="h-3.5 w-3.5" />
                    {duration}
                  </span>
                </>
              )}
              {result && (
                <>
                  <span>·</span>
                  <Badge
                    variant={result === 'pass' ? 'success' : 'destructive'}
                    className="text-xs"
                  >
                    {result}
                  </Badge>
                </>
              )}
            </div>
          </div>
        </div>
      </DialogHeader>

      <DialogBody className="max-h-[60vh]">
        {/* Toggle between messages and raw output */}
        {hasRawOutput && (
          <div className="flex items-center gap-2 mb-4">
            <button
              onClick={() => setShowRawOutput(false)}
              className={cn(
                'flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md transition-colors',
                !showRawOutput
                  ? 'bg-accent text-accent-foreground font-medium'
                  : 'text-muted-foreground hover:text-foreground hover:bg-accent/50'
              )}
            >
              <MessageSquare className="h-3.5 w-3.5" />
              Messages
            </button>
            <button
              onClick={() => setShowRawOutput(true)}
              className={cn(
                'flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md transition-colors',
                showRawOutput
                  ? 'bg-accent text-accent-foreground font-medium'
                  : 'text-muted-foreground hover:text-foreground hover:bg-accent/50'
              )}
            >
              <FileText className="h-3.5 w-3.5" />
              Raw Output
              {session?.raw_output_size ? (
                <span className="text-xs opacity-70">({formatBytes(session.raw_output_size)})</span>
              ) : null}
            </button>
          </div>
        )}

        {showRawOutput ? (
          // Raw output view
          rawOutputLoading ? (
            <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
              <Loader2 className="h-8 w-8 mb-3 animate-spin opacity-50" />
              <p className="text-sm">Loading raw output...</p>
            </div>
          ) : rawOutputData?.raw_output ? (
            <pre className="text-xs font-mono whitespace-pre-wrap break-all bg-muted/50 rounded-lg p-4 overflow-auto max-h-[50vh]">
              {rawOutputData.raw_output}
            </pre>
          ) : (
            <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
              <FileText className="h-12 w-12 mb-3 opacity-30" />
              <p className="text-sm">No raw output available</p>
            </div>
          )
        ) : (
          // Messages view
          messagesLoading ? (
            <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
              <Loader2 className="h-8 w-8 mb-3 animate-spin opacity-50" />
              <p className="text-sm">Loading messages...</p>
            </div>
          ) : messages.length > 0 ? (
            <div className="space-y-3">
              <div className="flex items-center gap-2 text-sm text-muted-foreground mb-4">
                <MessageSquare className="h-4 w-4" />
                <span>
                  {messagesData ? `${messagesData.total} total messages` : `${messages.length} message${messages.length !== 1 ? 's' : ''}`}
                </span>
              </div>

              <div className="space-y-2">
                <div ref={messagesStartRef} />
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
            </div>
          ) : (
            <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
              <MessageSquare className="h-12 w-12 mb-3 opacity-30" />
              <p className="text-sm">No messages available</p>
            </div>
          )
        )}
      </DialogBody>
    </Dialog>
  )
}
