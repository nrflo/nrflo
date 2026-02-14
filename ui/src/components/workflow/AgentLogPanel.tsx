import { useRef, useEffect, useMemo } from 'react'
import { ChevronRight, ChevronLeft, Loader2, MessageSquare, RefreshCw } from 'lucide-react'
import { cn, contextLeftColor } from '@/lib/utils'
import { useSessionMessages } from '@/hooks/useTickets'
import { parseToolName, ToolBadge } from './LogMessage'
import { AgentLogDetail, formatTime } from './AgentLogDetail'
import { Spinner } from '@/components/ui/Spinner'
import type { ActiveAgentV4, AgentSession, MessageWithTime } from '@/types/workflow'
import type { SelectedAgentData } from './PhaseGraph/types'

interface AgentMessagesBlockProps {
  agent: ActiveAgentV4
  session?: AgentSession
  onAgentClick: (agent: ActiveAgentV4, session?: AgentSession) => void
  onRestart?: (sessionId: string) => void
  restartingSessionId?: string | null
  onRetryFailed?: (sessionId: string) => void
  retryingSessionId?: string | null
  workflowStatus?: string
}

function AgentMessagesBlock({ agent, session, onAgentClick, onRestart, restartingSessionId, onRetryFailed, retryingSessionId, workflowStatus }: AgentMessagesBlockProps) {
  const isRunning = !agent.result
  const sessionId = session?.id || agent.session_id
  const { data: messagesData } = useSessionMessages(sessionId, {
    isRunning,
  })

  const messages: MessageWithTime[] = useMemo(() => {
    if (messagesData?.messages) return messagesData.messages
    if (session?.last_messages) {
      return session.last_messages.map(content => ({ content, created_at: '' }))
    }
    return []
  }, [messagesData, session?.last_messages])

  const modelId = agent.model_id
  const modelName = modelId
    ? modelId.split('-').slice(-2).join('-') || modelId
    : agent.cli || agent.agent_type || 'agent'

  const displayMessages = useMemo(() => messages.slice(-20).reverse(), [messages])

  return (
    <div className="border-b border-border last:border-b-0">
      <div
        role="button"
        tabIndex={0}
        onClick={() => onAgentClick(agent, session)}
        onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onAgentClick(agent, session) }}}
        className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-muted/50 transition-colors cursor-pointer"
      >
        {isRunning && <Loader2 className="h-3.5 w-3.5 text-yellow-600 dark:text-yellow-400 animate-spin shrink-0" />}
        <span className="text-sm font-medium truncate">
          {agent.phase?.replace(/_/g, ' ')} — {modelName}
        </span>
        {agent.context_left != null && (
          <span className={cn(
            'text-[10px] font-mono px-1 py-0.5 rounded shrink-0',
            contextLeftColor(agent.context_left)
          )}>
            {agent.context_left}%
          </span>
        )}
        {onRestart && agent.session_id && !agent.result && (
          <button
            onClick={(e) => { e.stopPropagation(); onRestart(agent.session_id!) }}
            disabled={restartingSessionId === agent.session_id}
            title="Restart agent (save context, relaunch)"
            className="ml-auto p-1 rounded hover:bg-muted transition-colors shrink-0 disabled:opacity-50"
          >
            {restartingSessionId === agent.session_id ? (
              <Spinner size="sm" />
            ) : (
              <RefreshCw className="h-3.5 w-3.5 text-muted-foreground" />
            )}
          </button>
        )}
        {onRetryFailed && agent.session_id && agent.result === 'fail' && workflowStatus === 'failed' && (
          <button
            onClick={(e) => { e.stopPropagation(); onRetryFailed(agent.session_id!) }}
            disabled={!!retryingSessionId}
            title="Retry failed agent"
            className="ml-auto p-1 rounded hover:bg-muted transition-colors shrink-0 disabled:opacity-50"
          >
            {retryingSessionId === agent.session_id ? (
              <Spinner size="sm" />
            ) : (
              <RefreshCw className="h-3.5 w-3.5 text-red-500" />
            )}
          </button>
        )}
        <MessageSquare className={cn("h-3.5 w-3.5 text-muted-foreground shrink-0", !onRestart && "ml-auto")} />
      </div>
      {displayMessages.length > 0 && (
        <div className="px-3 pb-2">
          <table className="w-full text-xs font-mono border-collapse">
            <thead>
              <tr className="text-left text-muted-foreground border-b border-border">
                <th className="py-1 pr-2 font-medium w-[70px]">Time</th>
                <th className="py-1 pr-2 font-medium w-[70px]">Tool</th>
                <th className="py-1 font-medium">Message</th>
              </tr>
            </thead>
            <tbody>
              {displayMessages.map((msg, i) => {
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
      )}
    </div>
  )
}

interface AgentLogPanelProps {
  activeAgents: Record<string, ActiveAgentV4>
  sessions: AgentSession[]
  collapsed: boolean
  onToggleCollapse: () => void
  selectedAgent: SelectedAgentData | null
  onAgentSelect: (data: SelectedAgentData | null) => void
  onRestart?: (sessionId: string) => void
  restartingSessionId?: string | null
  onRetryFailed?: (sessionId: string) => void
  retryingSessionId?: string | null
  workflowStatus?: string
}

export function AgentLogPanel({
  activeAgents,
  sessions,
  collapsed,
  onToggleCollapse,
  selectedAgent,
  onAgentSelect,
  onRestart,
  restartingSessionId,
  onRetryFailed,
  retryingSessionId,
  workflowStatus,
}: AgentLogPanelProps) {
  const scrollRef = useRef<HTMLDivElement>(null)

  const runningAgents = useMemo(() => {
    return Object.values(activeAgents).filter(a => !a.result)
  }, [activeAgents])

  const runningCount = runningAgents.length

  const findSession = (agent: ActiveAgentV4): AgentSession | undefined => {
    return sessions.find(s =>
      s.agent_type === agent.agent_type &&
      s.phase === agent.phase &&
      (!agent.model_id || s.model_id === agent.model_id)
    )
  }

  // Auto-scroll to top when new agents appear (overview mode only)
  useEffect(() => {
    if (!selectedAgent && scrollRef.current) {
      scrollRef.current.scrollTop = 0
    }
  }, [runningCount, selectedAgent])

  // When clicking a running agent in overview, open it in detail view
  const handleRunningAgentClick = (agent: ActiveAgentV4, session?: AgentSession) => {
    onAgentSelect({
      phaseName: agent.phase || agent.agent_type || '',
      agent,
      session,
    })
  }

  // Detail mode: show selected agent
  if (selectedAgent) {
    // Look up the latest session for live updates
    const liveSession = selectedAgent.agent
      ? findSession(selectedAgent.agent) || selectedAgent.session
      : selectedAgent.session
    const agentWithSession = { ...selectedAgent, session: liveSession }

    return (
      <div
        className={cn(
          'relative border-l border-border bg-background transition-all duration-300 ease-in-out shrink-0',
          collapsed ? 'w-10' : 'w-[280px]'
        )}
      >
        <button
          onClick={onToggleCollapse}
          className="absolute -left-5 top-3 z-10 flex items-center justify-center w-6 h-6 rounded-full border bg-background shadow-sm hover:bg-muted transition-colors"
          title={collapsed ? 'Expand agent log' : 'Collapse agent log'}
        >
          {collapsed ? <ChevronLeft className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
        </button>

        {collapsed ? (
          <div className="flex flex-col items-center pt-16 gap-2">
            <MessageSquare className="h-4 w-4 text-muted-foreground" />
            <span className="text-xs text-muted-foreground [writing-mode:vertical-lr] rotate-180">
              Agent Log
            </span>
          </div>
        ) : (
          <AgentLogDetail selectedAgent={agentWithSession} onBack={() => onAgentSelect(null)} />
        )}
      </div>
    )
  }

  // Overview mode: show running agents
  if (runningCount === 0) return null

  return (
    <div
      className={cn(
        'relative border-l border-border bg-background transition-all duration-300 ease-in-out shrink-0',
        collapsed ? 'w-10' : 'w-[280px]'
      )}
    >
      <button
        onClick={onToggleCollapse}
        className="absolute -left-5 top-3 z-10 flex items-center justify-center w-6 h-6 rounded-full border bg-background shadow-sm hover:bg-muted transition-colors"
        title={collapsed ? 'Expand agent log' : 'Collapse agent log'}
      >
        {collapsed ? <ChevronLeft className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
      </button>

      {collapsed ? (
        <div className="flex flex-col items-center pt-16 gap-2">
          <div className="flex items-center justify-center w-6 h-6 rounded-full bg-yellow-100 dark:bg-yellow-900/30 text-xs font-medium text-yellow-700 dark:text-yellow-400">
            {runningCount}
          </div>
          <span className="text-xs text-muted-foreground [writing-mode:vertical-lr] rotate-180">
            Agent Log
          </span>
        </div>
      ) : (
        <div className="flex flex-col h-full">
          <div className="flex items-center gap-2 px-3 py-2 border-b border-border shrink-0">
            <Loader2 className="h-3.5 w-3.5 text-yellow-600 dark:text-yellow-400 animate-spin" />
            <span className="text-sm font-medium">
              Running Agents ({runningCount})
            </span>
          </div>
          <div ref={scrollRef} className="flex-1 overflow-y-auto">
            {runningAgents.map((agent) => {
              const key = `${agent.agent_type}-${agent.model_id}-${agent.phase}`
              const session = findSession(agent)
              return (
                <AgentMessagesBlock
                  key={key}
                  agent={agent}
                  session={session}
                  onAgentClick={handleRunningAgentClick}
                  onRestart={onRestart}
                  restartingSessionId={restartingSessionId}
                  onRetryFailed={onRetryFailed}
                  retryingSessionId={retryingSessionId}
                  workflowStatus={workflowStatus}
                />
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
