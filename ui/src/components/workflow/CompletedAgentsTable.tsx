import { useState, useMemo } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { formatDateTime, formatElapsedTime } from '@/lib/utils'
import type { AgentHistoryEntry, AgentSession } from '@/types/workflow'
import type { SelectedAgentData } from './PhaseGraph/types'

const PAGE_SIZE = 20

function formatDuration(durationSec?: number): string {
  if (!durationSec) return '0s'
  const mins = Math.floor(durationSec / 60)
  const secs = durationSec % 60
  if (mins > 0) return secs > 0 ? `${mins}m ${secs}s` : `${mins}m`
  return `${secs}s`
}

function displayDuration(entry: AgentHistoryEntry): string {
  if (entry.started_at && entry.ended_at) {
    return formatElapsedTime(entry.started_at, entry.ended_at)
  }
  return formatDuration(entry.duration_sec)
}

interface CompletedAgentsTableProps {
  agentHistory: AgentHistoryEntry[]
  sessions: AgentSession[]
  onAgentSelect: (data: SelectedAgentData) => void
  showWorkflowColumn?: boolean
}

export function CompletedAgentsTable({ agentHistory, sessions, onAgentSelect, showWorkflowColumn }: CompletedAgentsTableProps) {
  const [currentPage, setCurrentPage] = useState(0)

  const sortedAgents = useMemo(() => {
    return [...agentHistory].sort((a, b) => {
      if (!a.ended_at && !b.ended_at) return 0
      if (!a.ended_at) return 1
      if (!b.ended_at) return -1
      return new Date(b.ended_at).getTime() - new Date(a.ended_at).getTime()
    })
  }, [agentHistory])

  const pageCount = Math.max(1, Math.ceil(sortedAgents.length / PAGE_SIZE))
  const safePage = Math.min(currentPage, pageCount - 1)
  const pageItems = sortedAgents.slice(safePage * PAGE_SIZE, (safePage + 1) * PAGE_SIZE)

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
    <div>
      <table className="w-full text-xs font-mono border-collapse">
        <thead>
          <tr className="text-left text-muted-foreground border-b border-border">
            {showWorkflowColumn && <th className="py-1.5 pr-3 font-medium">Workflow</th>}
            <th className="py-1.5 pr-3 font-medium">Agent</th>
            <th className="py-1.5 pr-3 font-medium">Phase</th>
            <th className="py-1.5 pr-3 font-medium">Model</th>
            <th className="py-1.5 pr-3 font-medium">Result</th>
            <th className="py-1.5 pr-3 font-medium">Duration</th>
            <th className="py-1.5 font-medium">Completed At</th>
          </tr>
        </thead>
        <tbody>
          {pageItems.map((entry, i) => (
            <tr
              key={`${entry.session_id || 'agent'}-${safePage}-${i}`}
              onClick={() => handleClick(entry)}
              className="border-b border-border/50 hover:bg-muted/50 cursor-pointer transition-colors"
            >
              {showWorkflowColumn && (
                <td className="py-1.5 pr-3 text-muted-foreground">
                  {(entry as AgentHistoryEntry & { workflow_label?: string }).workflow_label ?? '-'}
                </td>
              )}
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
                {displayDuration(entry)}
              </td>
              <td className="py-1.5 text-muted-foreground">
                {entry.ended_at ? formatDateTime(entry.ended_at) : '-'}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {pageCount > 1 && (
        <div className="flex items-center justify-between pt-3 text-xs text-muted-foreground">
          <span>
            {safePage * PAGE_SIZE + 1}–{Math.min((safePage + 1) * PAGE_SIZE, sortedAgents.length)} of {sortedAgents.length}
          </span>
          <div className="flex gap-1">
            <Button
              variant="outline"
              size="sm"
              disabled={safePage === 0}
              onClick={() => setCurrentPage(p => p - 1)}
              className="h-7 w-7 p-0"
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={safePage >= pageCount - 1}
              onClick={() => setCurrentPage(p => p + 1)}
              className="h-7 w-7 p-0"
            >
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
