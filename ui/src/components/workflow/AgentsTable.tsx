import { useMemo } from 'react'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { StatusCell } from '@/components/ui/StatusCell'
import { formatElapsedTime } from '@/lib/utils'
import { useTickingClock } from '@/hooks/useElapsedTime'
import { findSession } from './agentRowHelpers'
import type { PhaseState, ActiveAgentV4, AgentHistoryEntry, AgentSession } from '@/types/workflow'
import type { SelectedAgentData } from './PhaseGraph/types'

interface AgentsTableProps {
  phases: Record<string, PhaseState>
  activeAgents: Record<string, ActiveAgentV4>
  agentHistory?: AgentHistoryEntry[]
  phaseOrder?: string[]
  phaseLayers?: Record<string, number>
  sessions?: AgentSession[]
  onAgentSelect?: (data: SelectedAgentData) => void
}

interface AgentRow {
  phaseName: string
  layer: number
  orderIndex: number
  active?: ActiveAgentV4
  history?: AgentHistoryEntry
  session?: AgentSession
  status: string
}

export function AgentsTable({
  phases,
  activeAgents,
  agentHistory = [],
  phaseOrder = [],
  phaseLayers = {},
  sessions = [],
  onAgentSelect,
}: AgentsTableProps) {
  const hasRunning = Object.values(activeAgents).some(a => !a.result)
  useTickingClock(hasRunning)

  const rows = useMemo<AgentRow[]>(() => {
    return Object.keys(phases)
      .map(phaseName => {
        const active = Object.values(activeAgents).find(a => a.phase === phaseName)
        const phaseHistory = agentHistory
          .filter(h => h.phase === phaseName)
          .sort((a, b) => {
            if (!a.ended_at && !b.ended_at) return 0
            if (!a.ended_at) return 1
            if (!b.ended_at) return -1
            return new Date(b.ended_at).getTime() - new Date(a.ended_at).getTime()
          })
        const history = phaseHistory[0]
        const session = history ? findSession(history, sessions) : undefined

        let status: string
        if (active && !active.result) {
          status = 'running'
        } else if (history?.result === 'pass') {
          status = 'completed'
        } else if (history?.result === 'fail') {
          status = 'failed'
        } else if (history?.result === 'skipped') {
          status = 'skipped'
        } else {
          status = 'pending'
        }

        return {
          phaseName,
          layer: phaseLayers[phaseName] ?? 0,
          orderIndex: phaseOrder.indexOf(phaseName),
          active,
          history,
          session,
          status,
        }
      })
      .sort((a, b) => {
        if (a.layer !== b.layer) return a.layer - b.layer
        const ai = a.orderIndex < 0 ? Infinity : a.orderIndex
        const bi = b.orderIndex < 0 ? Infinity : b.orderIndex
        return ai - bi
      })
  }, [phases, activeAgents, agentHistory, sessions, phaseLayers, phaseOrder])

  const handleClick = (row: AgentRow) => {
    if (!onAgentSelect) return
    if (!row.active && !row.history) {
      onAgentSelect({ phaseName: row.phaseName })
    } else {
      onAgentSelect({
        phaseName: row.phaseName,
        agent: row.active,
        historyEntry: row.history,
        session: row.session,
      })
    }
  }

  const getDuration = (row: AgentRow): string => {
    if (row.active && row.active.started_at && !row.active.ended_at) {
      return formatElapsedTime(row.active.started_at)
    }
    if (row.history?.started_at && row.history.ended_at) {
      return formatElapsedTime(row.history.started_at, row.history.ended_at)
    }
    return '-'
  }

  const getModel = (row: AgentRow): string =>
    row.active?.model_id ?? row.history?.model_id ?? '-'

  const getContextLeft = (row: AgentRow): string => {
    const val = row.active?.context_left ?? row.history?.context_left
    return val != null ? `${val}%` : '-'
  }

  const getAttempts = (row: AgentRow): string => {
    const count = row.active?.restart_count ?? row.history?.restart_count
    return count != null ? String(count + 1) : '-'
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Agent</TableHead>
          <TableHead className="w-16">Level</TableHead>
          <TableHead className="w-32">Model</TableHead>
          <TableHead className="w-32">Status</TableHead>
          <TableHead className="w-20">Attempts</TableHead>
          <TableHead className="w-24">Context left</TableHead>
          <TableHead className="w-24">Duration</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map(row => (
          <TableRow
            key={row.phaseName}
            onClick={() => handleClick(row)}
            className="cursor-pointer"
          >
            <TableCell>
              <span className="text-primary hover:underline">
                {row.phaseName.replace(/_/g, ' ')}
              </span>
            </TableCell>
            <TableCell className="text-muted-foreground">{row.layer}</TableCell>
            <TableCell className="text-muted-foreground text-xs">{getModel(row)}</TableCell>
            <TableCell>
              <StatusCell status={row.status} />
            </TableCell>
            <TableCell className="text-muted-foreground">{getAttempts(row)}</TableCell>
            <TableCell className="text-muted-foreground">{getContextLeft(row)}</TableCell>
            <TableCell className="text-muted-foreground">{getDuration(row)}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
