import { useMemo } from 'react'
import { Badge } from '@/components/ui/Badge'
import { formatDateTime } from '@/lib/utils'
import type { AgentHistoryEntry, AgentSession } from '@/types/workflow'
import type { SelectedAgentData } from './PhaseGraph/types'

function formatDuration(durationSec?: number): string {
  if (!durationSec) return '0s'
  const mins = Math.floor(durationSec / 60)
  const secs = durationSec % 60
  if (mins > 0) return secs > 0 ? `${mins}m ${secs}s` : `${mins}m`
  return `${secs}s`
}

interface CompletedAgentsTableProps {
  agentHistory: AgentHistoryEntry[]
  sessions: AgentSession[]
  onAgentSelect: (data: SelectedAgentData) => void
}

export function CompletedAgentsTable({ agentHistory, sessions, onAgentSelect }: CompletedAgentsTableProps) {
  const sortedAgents = useMemo(() => {
    return [...agentHistory].sort((a, b) => {
      if (!a.ended_at && !b.ended_at) return 0
      if (!a.ended_at) return 1
      if (!b.ended_at) return -1
      return new Date(b.ended_at).getTime() - new Date(a.ended_at).getTime()
    })
  }, [agentHistory])

  const findSession = (entry: AgentHistoryEntry): AgentSession | undefined => {
    if (entry.session_id) {
      const byId = sessions.find(s => s.id === entry.session_id)
      if (byId) return byId
    }
    return sessions.find(s =>
      s.agent_type === entry.agent_type &&
      s.phase === entry.phase &&
      (!entry.model_id || s.model_id === entry.model_id)
    )
  }

  const handleClick = (entry: AgentHistoryEntry) => {
    const session = findSession(entry)
    onAgentSelect({
      phaseName: entry.phase,
      historyEntry: entry,
      session,
    })
  }

  if (sortedAgents.length === 0) {
    return (
      <p className="text-muted-foreground text-sm py-4">
        No completed agents
      </p>
    )
  }

  return (
    <table className="w-full text-xs font-mono border-collapse">
      <thead>
        <tr className="text-left text-muted-foreground border-b border-border">
          <th className="py-1.5 pr-3 font-medium">Agent</th>
          <th className="py-1.5 pr-3 font-medium">Phase</th>
          <th className="py-1.5 pr-3 font-medium">Model</th>
          <th className="py-1.5 pr-3 font-medium">Result</th>
          <th className="py-1.5 pr-3 font-medium">Duration</th>
          <th className="py-1.5 font-medium">Completed At</th>
        </tr>
      </thead>
      <tbody>
        {sortedAgents.map((entry, i) => (
          <tr
            key={`${entry.session_id || 'agent'}-${i}`}
            onClick={() => handleClick(entry)}
            className="border-b border-border/50 hover:bg-muted/50 cursor-pointer transition-colors"
          >
            <td className="py-1.5 pr-3">{entry.agent_type}</td>
            <td className="py-1.5 pr-3">{entry.phase.replace(/_/g, ' ')}</td>
            <td className="py-1.5 pr-3 text-muted-foreground">{entry.model_id ?? '-'}</td>
            <td className="py-1.5 pr-3">
              {entry.result && (
                <Badge
                  variant={entry.result === 'pass' ? 'success' : 'destructive'}
                  className="text-[10px] px-1 py-0"
                >
                  {entry.result}
                </Badge>
              )}
            </td>
            <td className="py-1.5 pr-3 text-muted-foreground">
              {formatDuration(entry.duration_sec)}
            </td>
            <td className="py-1.5 text-muted-foreground">
              {entry.ended_at ? formatDateTime(entry.ended_at) : '-'}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
