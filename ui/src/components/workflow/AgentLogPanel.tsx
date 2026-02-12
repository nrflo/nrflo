import { useRef, useEffect, useMemo } from 'react'
import { ChevronRight, ChevronLeft, Loader2, MessageSquare } from 'lucide-react'
import { cn, contextLeftColor } from '@/lib/utils'
import { useSessionMessages } from '@/hooks/useTickets'
import { LogMessage } from './LogMessage'
import { AgentLogDetail } from './AgentLogDetail'
import type { ActiveAgentV4, AgentSession, MessageWithTime } from '@/types/workflow'
import type { SelectedAgentData } from './PhaseGraph/types'

interface AgentMessagesBlockProps {
  agent: ActiveAgentV4
  session?: AgentSession
  onAgentClick: (agent: ActiveAgentV4, session?: AgentSession) => void
}

function AgentMessagesBlock({ agent, session, onAgentClick }: AgentMessagesBlockProps) {
  const isRunning = !agent.result
  const { data: messagesData } = useSessionMessages(session?.id, {
    enabled: !!session?.id,
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

  const displayMessages = useMemo(() => [...messages].reverse().slice(0, 20), [messages])

  return (
    <div className="border-b border-border last:border-b-0">
      <button
        onClick={() => onAgentClick(agent, session)}
        className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-muted/50 transition-colors"
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
        <MessageSquare className="h-3.5 w-3.5 text-muted-foreground ml-auto shrink-0" />
      </button>
      {displayMessages.length > 0 && (
        <div className="px-3 pb-2 space-y-1">
          {displayMessages.map((msg, i) => (
            <LogMessage
              key={i}
              message={msg.content}
              variant="compact"
            />
          ))}
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
}

export function AgentLogPanel({
  activeAgents,
  sessions,
  collapsed,
  onToggleCollapse,
  selectedAgent,
  onAgentSelect,
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
          collapsed ? 'w-10' : 'flex-1 min-w-[300px]'
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
        collapsed ? 'w-10' : 'flex-1 min-w-[300px]'
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
                />
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
