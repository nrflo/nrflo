import { ChevronDown, ChevronRight } from 'lucide-react'
import { cn, formatElapsedTime, contextLeftColor } from '@/lib/utils'
import { ResultIcon, formatDuration } from './resultUtils'
import type { AgentHistoryEntry } from '@/types/workflow'

interface AgentBadgeProps {
  agent: AgentHistoryEntry
  findings?: Record<string, unknown>
  expanded: boolean
  onToggle: () => void
}

export function AgentBadge({ agent, findings, expanded, onToggle }: AgentBadgeProps) {
  const hasFindings = findings && Object.keys(findings).length > 0

  const elapsed = agent.started_at && agent.ended_at
    ? formatElapsedTime(agent.started_at, agent.ended_at)
    : agent.duration_sec
      ? formatDuration(agent.duration_sec)
      : undefined

  const badgeClass = cn(
    'inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-mono transition-colors',
    agent.result === 'pass' && 'bg-green-100 text-green-800 dark:bg-green-900/50 dark:text-green-200',
    agent.result === 'fail' && 'bg-red-100 text-red-800 dark:bg-red-900/50 dark:text-red-200',
    !agent.result && 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300',
    hasFindings && 'cursor-pointer hover:opacity-80'
  )

  return (
    <span
      role="button"
      tabIndex={hasFindings ? 0 : -1}
      className={badgeClass}
      onClick={(e) => {
        e.stopPropagation()
        if (hasFindings) onToggle()
      }}
      onKeyDown={(e) => {
        if (hasFindings && (e.key === 'Enter' || e.key === ' ')) {
          e.preventDefault()
          e.stopPropagation()
          onToggle()
        }
      }}
      title={hasFindings ? `${agent.agent_type} - click to ${expanded ? 'collapse' : 'expand'} findings` : agent.agent_type}
    >
      {hasFindings && (
        expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />
      )}
      <span>{agent.agent_type}</span>
      {(agent.restart_count ?? 0) > 0 && (
        <span className="text-red-600 dark:text-red-400">↻{agent.restart_count}</span>
      )}
      {agent.result && <ResultIcon result={agent.result} />}
      {elapsed && (
        <span className="text-[10px] opacity-70">{elapsed}</span>
      )}
      {agent.context_left != null && (
        <span className={cn(
          'text-[10px] px-1 rounded',
          contextLeftColor(agent.context_left)
        )}>
          {agent.context_left}%
        </span>
      )}
    </span>
  )
}
