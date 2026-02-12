import { useState, useCallback } from 'react'
import { ChevronDown, ChevronRight, Cpu } from 'lucide-react'
import { cn, statusColor, formatDateTime, contextLeftColor } from '@/lib/utils'
import { Badge } from '@/components/ui/Badge'
import type { PhaseState, WorkflowFindings, ActiveAgentV4, AgentHistoryEntry } from '@/types/workflow'
import { resultColor, ResultIcon } from './resultUtils'
import { resolveAgentFindings } from './findingUtils'
import { AgentBadge } from './AgentBadge'
import { AgentHistoryCard, InlineFindings } from './AgentHistoryCard'

interface PhaseCardProps {
  name: string
  phase: PhaseState
  isCurrent?: boolean
  findings?: WorkflowFindings
  activeAgents?: Record<string, ActiveAgentV4>
  agentHistory?: AgentHistoryEntry[]
}

// Reverse mapping: agent type to phase
function agentTypeToPhase(agentType: string): string | null {
  const mapping: Record<string, string> = {
    'setup-analyzer': 'investigation',
    'test-writer': 'test-design',
    'implementor': 'implementation',
    'qa-verifier': 'verification',
    'doc-updater': 'docs',
  }
  return mapping[agentType] || null
}

// Mapping: phase name -> agent types that run in that phase
const phaseAgentMapping: Record<string, string[]> = {
  'investigation': ['setup-analyzer'],
  'test-design': ['test-writer'],
  'implementation': ['implementor'],
  'verification': ['test-writer', 'qa-verifier'],
  'docs': ['doc-updater'],
}

export function PhaseCard({ name, phase, isCurrent, findings, activeAgents, agentHistory }: PhaseCardProps) {
  const [expanded, setExpanded] = useState(isCurrent || phase.status === 'error')
  const [expandedAgents, setExpandedAgents] = useState<Set<string>>(new Set())
  const hasDetails = phase.error || phase.started_at || phase.ended_at || phase.result

  // Filter agent history for this phase
  let phaseAgentHistory = agentHistory?.filter(h => h.phase === name) || []

  // Fallback: if no agent_history but we have findings, derive agents from findings
  if (phaseAgentHistory.length === 0 && findings) {
    const expectedAgentTypes = phaseAgentMapping[name] || []
    phaseAgentHistory = expectedAgentTypes
      .filter(agentType => findings[agentType] && Object.keys(findings[agentType]).length > 0)
      .map(agentType => ({
        agent_id: `${name}-${agentType}`,
        agent_type: agentType,
        phase: name,
        result: phase.result === 'pass' ? 'pass' : phase.result === 'fail' ? 'fail' : undefined,
      }))
  }

  const hasAgentHistory = phaseAgentHistory.length > 0

  const phaseHasFindings = findings && phaseAgentHistory.some(agent => {
    const agentFindings = resolveAgentFindings(findings, agent.agent_type, agent.model_id)
    return agentFindings && Object.keys(agentFindings).length > 0
  })

  // Filter active agents for this phase
  const phaseActiveAgents = activeAgents
    ? Object.entries(activeAgents).filter(([, agent]) => {
        const agentPhase = agentTypeToPhase(agent.agent_type)
        return agentPhase === name
      })
    : []
  const hasActiveAgents = phaseActiveAgents.length > 0

  const canExpand = hasDetails || hasAgentHistory || hasActiveAgents || phaseHasFindings

  const toggleAgentExpansion = useCallback((agentId: string) => {
    setExpandedAgents(prev => {
      const next = new Set(prev)
      if (next.has(agentId)) {
        next.delete(agentId)
      } else {
        next.add(agentId)
      }
      return next
    })
  }, [])

  return (
    <div
      className={cn(
        'rounded-lg border p-3',
        isCurrent && 'border-primary bg-primary/5',
        phase.status === 'error' && 'border-red-500 bg-red-50 dark:bg-red-950'
      )}
    >
      <div className="space-y-0">
        <button
          className={cn(
            'flex items-center gap-2 w-full text-left',
            canExpand && 'cursor-pointer'
          )}
          onClick={() => canExpand && setExpanded(!expanded)}
          disabled={!canExpand}
        >
          {canExpand && (
            <span className="text-muted-foreground">
              {expanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
            </span>
          )}
          <h3 className="font-medium capitalize">
            {name.replace(/_/g, ' ')}
          </h3>
          {hasAgentHistory && (
            <div className="flex items-center gap-1 flex-wrap">
              {phaseAgentHistory.map((agent) => (
                <AgentBadge
                  key={agent.agent_id}
                  agent={agent}
                  findings={resolveAgentFindings(findings, agent.agent_type, agent.model_id)}
                  expanded={expandedAgents.has(agent.agent_id)}
                  onToggle={() => toggleAgentExpansion(agent.agent_id)}
                />
              ))}
            </div>
          )}
          <div className="flex items-center gap-2 ml-auto">
            {phase.result && (
              <Badge className={cn('text-xs flex items-center gap-1', resultColor(phase.result))}>
                <ResultIcon result={phase.result} />
                {phase.result}
              </Badge>
            )}
            <Badge className={cn('text-xs', statusColor(phase.status))}>
              {phase.status.replace(/_/g, ' ')}
            </Badge>
          </div>
        </button>

        {expandedAgents.size > 0 && (
          <div className="pl-6 space-y-2">
            {phaseAgentHistory
              .filter(agent => expandedAgents.has(agent.agent_id))
              .map(agent => {
                const agentFindings = resolveAgentFindings(findings, agent.agent_type, agent.model_id)
                if (!agentFindings || Object.keys(agentFindings).length === 0) return null
                return (
                  <InlineFindings
                    key={agent.agent_id}
                    agent={agent}
                    findings={agentFindings}
                  />
                )
              })}
          </div>
        )}
      </div>

      {expanded && canExpand && (
        <div className="mt-3 pt-3 border-t border-border/50 space-y-3">
          <div className="flex flex-wrap gap-x-6 gap-y-2 text-sm">
            {phase.result && (
              <div className="flex items-center gap-2">
                <span className="text-muted-foreground">Result:</span>
                <Badge className={cn('text-xs flex items-center gap-1', resultColor(phase.result))}>
                  <ResultIcon result={phase.result} />
                  {phase.result}
                </Badge>
              </div>
            )}
            {phase.started_at && (
              <div>
                <span className="text-muted-foreground">Started:</span>{' '}
                <span className="text-xs">{formatDateTime(phase.started_at)}</span>
              </div>
            )}
            {phase.ended_at && (
              <div>
                <span className="text-muted-foreground">Ended:</span>{' '}
                <span className="text-xs">{formatDateTime(phase.ended_at)}</span>
              </div>
            )}
          </div>

          {phase.error && (
            <div className="p-2 bg-red-100 dark:bg-red-900/50 rounded text-sm text-red-800 dark:text-red-200">
              <strong>Error:</strong> {phase.error}
            </div>
          )}

          {hasActiveAgents && (
            <div className="space-y-2">
              <div className="text-xs font-medium text-muted-foreground">Running:</div>
              {phaseActiveAgents.map(([key, agent]) => (
                <div
                  key={key}
                  className="flex items-center gap-3 p-2 rounded bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800"
                >
                  <Cpu className="h-4 w-4 text-yellow-600 dark:text-yellow-400 animate-pulse shrink-0" />
                  <div className="flex-1 min-w-0">
                    <div className="font-mono text-xs">{agent.agent_type}</div>
                    {agent.model && (
                      <div className="text-xs text-muted-foreground truncate">{agent.model}</div>
                    )}
                  </div>
                  <div className="flex items-center gap-2 text-xs text-muted-foreground text-right">
                    {agent.context_left != null && (
                      <span className={cn(
                        'font-mono px-1.5 py-0.5 rounded',
                        contextLeftColor(agent.context_left)
                      )}>
                        {agent.context_left}%
                      </span>
                    )}
                    <div>
                      {agent.pid && <div>PID: {agent.pid}</div>}
                      {agent.started_at && <div>{formatDateTime(agent.started_at)}</div>}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}

          {hasAgentHistory && (
            <div className="space-y-2">
              <div className="text-xs font-medium text-muted-foreground">Agents:</div>
              {phaseAgentHistory.map((agent) => (
                <AgentHistoryCard
                  key={agent.agent_id}
                  agent={agent}
                  findings={resolveAgentFindings(findings, agent.agent_type, agent.model_id)}
                />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
