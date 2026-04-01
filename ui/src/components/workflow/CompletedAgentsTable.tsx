import { useState, useMemo } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
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
      <Table data-testid="agent-table">
        <TableHeader>
          <TableRow data-testid="agent-table-header">
            <TableHead>Agent</TableHead>
            <TableHead>Phase</TableHead>
            <TableHead className="w-28">Model</TableHead>
            <TableHead className="w-16">Result</TableHead>
            <TableHead className="w-20">Duration</TableHead>
            <TableHead>Completed At</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {pageItems.map((entry, i) => (
            <TableRow
              key={`${entry.session_id || 'agent'}-${safePage}-${i}`}
              onClick={() => handleClick(entry)}
              className="cursor-pointer hover:bg-muted/50"
              data-testid="agent-row"
            >
              <TableCell>{entry.agent_type}</TableCell>
              <TableCell>{entry.phase.replace(/_/g, ' ')}</TableCell>
              <TableCell className="text-muted-foreground">{entry.model_id ?? '-'}</TableCell>
              <TableCell>
                {entry.result && (
                  <Badge
                    variant={entry.result === 'pass' ? 'success' : 'destructive'}
                    className="text-[10px] px-1 py-0"
                  >
                    {entry.result}
                  </Badge>
                )}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {displayDuration(entry)}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {entry.ended_at ? formatDateTime(entry.ended_at) : '-'}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
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
