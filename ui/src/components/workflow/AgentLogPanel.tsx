import { useEffect, useMemo, useState } from 'react'
import { Loader2, MessageSquare } from 'lucide-react'
import { cn } from '@/lib/utils'
import { AgentLogDetail } from './AgentLogDetail'
import type { ActiveAgentV4, AgentSession, WorkflowFindings } from '@/types/workflow'
import type { SelectedAgentData } from './PhaseGraph/types'

interface AgentLogPanelProps {
  activeAgents: Record<string, ActiveAgentV4>
  sessions: AgentSession[]
  collapsed: boolean
  selectedAgent: SelectedAgentData | null
  onAgentSelect: (data: SelectedAgentData | null) => void
  onResumeSession?: (sessionId: string) => void
  resumePending?: boolean
  agentFindings?: WorkflowFindings
  projectFindings?: Record<string, unknown>
  phaseLayers?: Record<string, number>
  workflowFindings?: Record<string, unknown>
}

export function AgentLogPanel({
  activeAgents,
  sessions,
  collapsed,
  selectedAgent,
  onAgentSelect,
  onResumeSession,
  resumePending,
  agentFindings,
  projectFindings,
  phaseLayers,
  workflowFindings,
}: AgentLogPanelProps) {
  const runningAgents = useMemo(() => {
    return Object.values(activeAgents).filter(a => !a.result)
  }, [activeAgents])

  const findSession = (agent: ActiveAgentV4): AgentSession | undefined => {
    if (agent.session_id) {
      const byId = sessions.find(s => s.id === agent.session_id)
      if (byId) return byId
    }
    return sessions.find(s =>
      s.agent_type === agent.agent_type &&
      s.phase === agent.phase &&
      (!agent.model_id || s.model_id === agent.model_id)
    )
  }

  // Resolve live agent data from activeAgents to replace stale captured snapshot.
  const liveAgent = useMemo(() => {
    if (!selectedAgent?.agent) return undefined
    return Object.values(activeAgents).find(a =>
      a.session_id === selectedAgent.agent!.session_id ||
      (a.agent_type === selectedAgent.agent!.agent_type &&
       a.phase === selectedAgent.agent!.phase &&
       a.model_id === selectedAgent.agent!.model_id)
    ) ?? selectedAgent.agent
  }, [selectedAgent, activeAgents])

  // Auto-switch: when selected agent completes and other agents are running, return to all-running view
  const liveAgentResult = liveAgent?.result
  useEffect(() => {
    if (!liveAgentResult || runningAgents.length === 0) return
    onAgentSelect(null)
  }, [liveAgentResult, runningAgents]) // eslint-disable-line react-hooks/exhaustive-deps

  // Tab state for multi-agent view (hooks must be before early returns)
  const [activeTabKey, setActiveTabKey] = useState<string | null>(null)

  const agentTabKeys = useMemo(() =>
    runningAgents.map(a => `${a.agent_type}-${a.model_id}-${a.phase}`),
    [runningAgents]
  )

  // Auto-select first tab when current selection is stale or missing
  useEffect(() => {
    if (agentTabKeys.length === 0) return
    if (activeTabKey && agentTabKeys.includes(activeTabKey)) return
    setActiveTabKey(agentTabKeys[0])
  }, [agentTabKeys, activeTabKey])

  // Detail mode: show explicitly selected agent (from PhaseGraph click on history agent)
  if (selectedAgent) {
    const resolvedSelected = { ...selectedAgent, agent: liveAgent }
    const isRunningAgent = resolvedSelected.agent && !resolvedSelected.agent.result
    const liveSession = isRunningAgent
      ? findSession(resolvedSelected.agent!) || resolvedSelected.session
      : resolvedSelected.session
    const agentWithSession = { ...resolvedSelected, session: liveSession }

    return (
      <PanelShell collapsed={collapsed}>
        {collapsed ? (
          <CollapsedBar />
        ) : (
          <AgentLogDetail selectedAgent={agentWithSession} onBack={() => onAgentSelect(null)} onResumeSession={onResumeSession} resumePending={resumePending} agentFindings={agentFindings} projectFindings={projectFindings} phaseLayers={phaseLayers} workflowFindings={workflowFindings} />
        )}
      </PanelShell>
    )
  }

  // No selected agent — show all running agents in detail view
  if (runningAgents.length === 0) return null

  const selectedTabAgent = runningAgents.find((_, i) => agentTabKeys[i] === activeTabKey)

  return (
    <PanelShell collapsed={collapsed}>
      {collapsed ? (
        <CollapsedBar />
      ) : (
        <div className="flex flex-col h-full">
          <div className="flex items-center gap-1 px-3 py-1 border-b border-border shrink-0">
            {runningAgents.map((agent, i) => {
              const key = agentTabKeys[i]
              const label = (agent.phase || agent.agent_type || '').replace(/_/g, ' ')
              return (
                <button
                  key={key}
                  data-testid="agent-tab"
                  onClick={() => setActiveTabKey(key)}
                  className={cn(
                    'px-2.5 py-1 text-xs font-medium rounded transition-colors flex items-center gap-1.5',
                    activeTabKey === key
                      ? 'bg-muted text-foreground'
                      : 'text-muted-foreground hover:text-foreground hover:bg-muted/50',
                  )}
                >
                  <Loader2 className="h-3 w-3 spin-sync text-yellow-500" />
                  {label}
                </button>
              )
            })}
          </div>
          {selectedTabAgent && (() => {
            const session = findSession(selectedTabAgent)
            const agentData: SelectedAgentData = {
              phaseName: selectedTabAgent.phase || selectedTabAgent.agent_type || '',
              agent: selectedTabAgent,
              session,
            }
            return (
              <div className="flex-1 min-h-0 overflow-hidden">
                <AgentLogDetail selectedAgent={agentData} onResumeSession={onResumeSession} resumePending={resumePending} agentFindings={agentFindings} projectFindings={projectFindings} phaseLayers={phaseLayers} workflowFindings={workflowFindings} />
              </div>
            )
          })()}
        </div>
      )}
    </PanelShell>
  )
}

function PanelShell({ collapsed, children }: { collapsed: boolean; children: React.ReactNode }) {
  return (
    <div
      className={cn(
        'relative border-t md:border-t-0 md:border-l border-border bg-background transition-all duration-300 ease-in-out',
        collapsed ? 'h-10 md:h-auto md:w-10 shrink-0' : 'h-[50vh] md:h-auto md:flex-1 md:min-w-[280px]'
      )}
    >
      {children}
    </div>
  )
}

function CollapsedBar() {
  return (
    <div className="flex flex-row items-center gap-2 justify-center md:flex-col md:pt-16">
      <MessageSquare className="h-4 w-4 text-muted-foreground" />
      <span className="text-xs text-muted-foreground md:[writing-mode:vertical-lr] md:rotate-180">
        Agent Log
      </span>
    </div>
  )
}
