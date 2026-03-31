import { useState, useMemo } from 'react'
import { ArrowLeft, MessageSquare, Loader2, CheckCircle, XCircle, Cpu, Timer, Terminal } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import { Tooltip } from '@/components/ui/Tooltip'
import { parseToolName, ToolBadge } from './LogMessage'
import { cn } from '@/lib/utils'
import { useSessionMessages } from '@/hooks/useTickets'
import type { MessageCategory } from '@/types/workflow'
import type { SelectedAgentData } from './PhaseGraph/types'

const CATEGORY_TABS: { value: MessageCategory | 'all'; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'text', label: 'Text' },
  { value: 'tool', label: 'Tools' },
  { value: 'subagent', label: 'Sub-agents' },
  { value: 'skill', label: 'Skills' },
]

function formatDuration(durationSec?: number): string {
  if (!durationSec) return '0s'
  const mins = Math.floor(durationSec / 60)
  const secs = durationSec % 60
  if (mins > 0) return secs > 0 ? `${mins}m ${secs}s` : `${mins}m`
  return `${secs}s`
}

export function formatTime(dateStr: string): string {
  if (!dateStr) return ''
  const d = new Date(dateStr)
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

interface AgentLogDetailProps {
  selectedAgent: SelectedAgentData
  onBack: () => void
  onResumeSession?: (sessionId: string) => void
  resumePending?: boolean
}

export function AgentLogDetail({ selectedAgent, onBack, onResumeSession, resumePending }: AgentLogDetailProps) {
  const { agent, historyEntry, session, phaseName } = selectedAgent
  const isInteractive = session?.status === 'user_interactive'
  const isRunning = agent && !agent.result && !isInteractive
  const result = agent?.result || historyEntry?.result
  const modelId = agent?.model_id || historyEntry?.model_id
  const modelName = modelId
    ? modelId.split('-').slice(-2).join('-') || modelId
    : agent?.cli || historyEntry?.agent_type || 'agent'
  const duration = historyEntry?.duration_sec ? formatDuration(historyEntry.duration_sec) : null

  const [categoryFilter, setCategoryFilter] = useState<MessageCategory | 'all'>('all')
  const sessionId = session?.id || agent?.session_id || historyEntry?.session_id
  const { data: messagesData, isLoading: messagesLoading } = useSessionMessages(sessionId, {
    isRunning: isRunning || false,
  })

  const messages = messagesData?.messages ?? []

  const categoryCounts = useMemo(() => {
    const counts: Record<string, number> = { all: messages.length, text: 0, tool: 0, subagent: 0, skill: 0 }
    for (const m of messages) {
      const cat = m.category || 'text'
      counts[cat] = (counts[cat] || 0) + 1
    }
    return counts
  }, [messages])

  const filteredMessages = useMemo(
    () => categoryFilter === 'all' ? messages : messages.filter(m => (m.category || 'text') === categoryFilter),
    [messages, categoryFilter],
  )

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
          isInteractive && 'bg-blue-100 dark:bg-blue-900/30',
          isRunning && 'bg-yellow-100 dark:bg-yellow-900/30',
          result === 'pass' && 'bg-green-100 dark:bg-green-900/30',
          result === 'fail' && 'bg-red-100 dark:bg-red-900/30',
          !result && !isRunning && !isInteractive && 'bg-gray-100 dark:bg-gray-800'
        )}>
          {isInteractive && <Terminal className="h-3.5 w-3.5 text-blue-600 dark:text-blue-400" />}
          {isRunning && <Loader2 className="h-3.5 w-3.5 text-yellow-600 dark:text-yellow-400 spin-sync" />}
          {result === 'pass' && <CheckCircle className="h-3.5 w-3.5 text-green-600 dark:text-green-400" />}
          {result === 'fail' && <XCircle className="h-3.5 w-3.5 text-red-600 dark:text-red-400" />}
          {!result && !isRunning && !isInteractive && <Cpu className="h-3.5 w-3.5 text-muted-foreground" />}
        </div>
        <div className="min-w-0 flex-1">
          <div className="text-sm font-medium truncate">{phaseName.replace(/_/g, ' ')}</div>
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span>{modelName}</span>
            {isInteractive && (
              <>
                <span>·</span>
                <Badge className="text-[10px] px-1 py-0 bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400">
                  User controlling
                </Badge>
              </>
            )}
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
        {onResumeSession && sessionId && (result === 'pass' || result === 'fail') && modelId?.startsWith('claude:') && session?.status !== 'user_interactive' && (
          <Tooltip text="Resume this session in an interactive terminal" placement="top">
            <Button
              variant="outline"
              size="sm"
              onClick={() => onResumeSession(sessionId)}
              disabled={resumePending}
              className="text-blue-600 hover:text-blue-700 shrink-0"
            >
              {resumePending ? (
                <Spinner size="sm" className="mr-1.5" />
              ) : (
                <Terminal className="h-3.5 w-3.5 mr-1.5" />
              )}
              Resume
            </Button>
          </Tooltip>
        )}
      </div>

      {/* Content area */}
      <div className="flex-1 overflow-y-auto overflow-x-hidden px-3 py-2">
        {messagesLoading ? (
          <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
            <Loader2 className="h-6 w-6 mb-2 spin-sync opacity-50" />
            <p className="text-xs">Loading messages...</p>
          </div>
        ) : messages.length > 0 ? (
          <div>
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-2">
              <MessageSquare className="h-3 w-3" />
              <span>
                {categoryFilter !== 'all'
                  ? `${filteredMessages.length} of ${messages.length} messages`
                  : `${messages.length} messages`}
              </span>
            </div>
            <div className="flex items-center gap-0.5 mb-2 border-b border-border" role="tablist">
              {CATEGORY_TABS.map((tab) => (
                <button
                  key={tab.value}
                  role="tab"
                  aria-selected={categoryFilter === tab.value}
                  onClick={() => setCategoryFilter(tab.value)}
                  className={cn(
                    'px-2 py-1 text-xs font-medium transition-colors rounded-t',
                    categoryFilter === tab.value
                      ? 'border-b-2 border-primary text-foreground bg-muted'
                      : 'text-muted-foreground hover:text-foreground hover:bg-muted/50',
                  )}
                >
                  {tab.label}
                  <span className={cn(
                    'ml-1 px-1 py-0.5 rounded text-[10px]',
                    categoryFilter === tab.value
                      ? 'bg-primary/10 text-primary'
                      : 'bg-muted text-muted-foreground',
                  )}>
                    {categoryCounts[tab.value] ?? 0}
                  </span>
                </button>
              ))}
            </div>
            <table className="w-full table-fixed text-xs font-mono border-collapse">
              <thead>
                <tr className="text-left text-muted-foreground border-b border-border">
                  <th className="py-1 pr-2 font-medium w-[90px]">Time</th>
                  <th className="py-1 pr-2 font-medium w-[70px]">Tool</th>
                  <th className="py-1 font-medium">Message</th>
                </tr>
              </thead>
              <tbody>
                {[...filteredMessages].reverse().map((msg, i) => {
                  const { toolName, rest } = parseToolName(msg.content)
                  return (
                    <tr key={i} className="border-b border-border/50 align-top">
                      <td className="py-1 pr-2 text-muted-foreground whitespace-nowrap overflow-hidden text-ellipsis">
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
