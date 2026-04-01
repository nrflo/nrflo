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
}

export function CompletedAgentsTable({ agentHistory, sessions, onAgentSelect }: CompletedAgentsTableProps) {
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
      <div className="border border-border rounded-lg text-xs font-mono" data-testid="agent-table">
        <div className="px-4 py-2 border-b border-border bg-muted/30">
          <div className="flex items-center gap-4 text-xs font-medium text-muted-foreground uppercase tracking-wider" data-testid="agent-table-header">
            <span className="flex-1 shrink-0">Agent</span>
            <span className="flex-1 shrink-0">Phase</span>
            <span className="w-28 shrink-0">Model</span>
            <span className="w-16 shrink-0">Result</span>
            <span className="w-20 shrink-0">Duration</span>
            <span className="flex-1 shrink-0">Completed At</span>
          </div>
        </div>
        {pageItems.map((entry, i) => (
          <div
            key={`${entry.session_id || 'agent'}-${safePage}-${i}`}
            onClick={() => handleClick(entry)}
            className="flex items-center gap-4 px-4 py-3 border-b border-border last:border-b-0 hover:bg-muted/50 cursor-pointer transition-colors"
            data-testid="agent-row"
          >
            <span className="flex-1 shrink-0">{entry.agent_type}</span>
            <span className="flex-1 shrink-0">{entry.phase.replace(/_/g, ' ')}</span>
            <span className="w-28 shrink-0 text-muted-foreground">{entry.model_id ?? '-'}</span>
            <span className="w-16 shrink-0">
              {entry.result && (
                <Badge
                  variant={entry.result === 'pass' ? 'success' : 'destructive'}
                  className="text-[10px] px-1 py-0"
                >
                  {entry.result}
                </Badge>
              )}
            </span>
            <span className="w-20 shrink-0 text-muted-foreground">
              {displayDuration(entry)}
            </span>
            <span className="flex-1 shrink-0 text-muted-foreground">
              {entry.ended_at ? formatDateTime(entry.ended_at) : '-'}
            </span>
          </div>
        ))}
      </div>
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
