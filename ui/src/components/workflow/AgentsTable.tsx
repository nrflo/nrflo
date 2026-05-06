import { useState, useMemo } from 'react'
import { AlertTriangle, RefreshCw } from 'lucide-react'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { StatusCell } from '@/components/ui/StatusCell'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import { Tooltip } from '@/components/ui/Tooltip'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { cn, formatElapsedTime, formatRestartReasons, contextLeftColor } from '@/lib/utils'
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
  onRetryFailed?: (sessionId: string) => void
  retryingSessionId?: string | null
  workflowStatus?: string
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

function runModeLabel(active?: ActiveAgentV4, history?: AgentHistoryEntry): string {
  const mode = active?.effective_mode ?? history?.effective_mode
  if (!mode) return '—'
  if (mode === 'cli_interactive') return 'cli interactive'
  return mode
}

export function AgentsTable({
  phases,
  activeAgents,
  agentHistory = [],
  phaseOrder = [],
  phaseLayers = {},
  sessions = [],
  onAgentSelect,
  onRetryFailed,
  retryingSessionId,
  workflowStatus,
}: AgentsTableProps) {
  const [confirmSessionId, setConfirmSessionId] = useState<string | null>(null)
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

  const confirmRow = confirmSessionId
    ? rows.find(r => r.history?.session_id === confirmSessionId)
    : null

  return (
    <>
      <div className="pr-2 sm:pr-4">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Agent</TableHead>
              <TableHead className="w-16">Level</TableHead>
              <TableHead className="w-28">Model</TableHead>
              <TableHead className="w-24">Run mode</TableHead>
              <TableHead className="w-32">Status</TableHead>
              <TableHead className="w-20">Attempts</TableHead>
              <TableHead className="w-24">Context left</TableHead>
              <TableHead className="w-24">Duration</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map(row => {
              const rawRestartCount = row.active?.restart_count ?? row.history?.restart_count
              const restartCount = rawRestartCount ?? 0
              const restartDetails = row.active?.restart_details ?? row.history?.restart_details
              const ctxLeft = row.active?.context_left ?? row.history?.context_left
              const restartThreshold = row.active?.restart_threshold ?? row.history?.restart_threshold ?? 25
              const nudgeCount = row.active?.nudge_count ?? 0
              const isInteractive = row.session?.status === 'user_interactive'
              const showRetry = row.history?.result === 'fail' && workflowStatus === 'failed' && !!onRetryFailed && !!row.history?.session_id
              const tag = row.active?.tag ?? row.history?.tag

              return (
                <TableRow
                  key={row.phaseName}
                  onClick={() => handleClick(row)}
                  className="cursor-pointer"
                >
                  <TableCell>
                    <span className="text-primary hover:underline">
                      {row.phaseName.replace(/_/g, ' ')}
                    </span>
                    {tag && (
                      <Badge variant="outline" className="ml-2 text-xs border-emerald-300 text-emerald-600">
                        {tag}
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-muted-foreground text-xs">{row.layer}</TableCell>
                  <TableCell className="text-muted-foreground text-xs">{getModel(row)}</TableCell>
                  <TableCell className="text-muted-foreground text-xs">
                    {runModeLabel(row.active, row.history)}
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1 flex-wrap">
                      <StatusCell status={row.status} />
                      {isInteractive && (
                        <span className="text-[10px] text-blue-600 dark:text-blue-400">(interactive control)</span>
                      )}
                      {row.status === 'running' && nudgeCount > 0 && (
                        <Tooltip text="Idle reminder sent — agent has not called nrflo agent continue/fail" placement="top">
                          <span className="text-xs font-mono px-1 rounded bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400">
                            ⏰{nudgeCount}/5
                          </span>
                        </Tooltip>
                      )}
                      {showRetry && (
                        <Tooltip text="Retry failed agent" placement="top">
                          <button
                            onClick={(e) => { e.stopPropagation(); setConfirmSessionId(row.history!.session_id!) }}
                            disabled={!!retryingSessionId}
                            className="p-0.5 rounded-full bg-red-100 hover:bg-red-200 dark:bg-red-900/50 dark:hover:bg-red-800/50 transition-colors disabled:opacity-50 border border-red-300 dark:border-red-700"
                          >
                            {retryingSessionId === row.history!.session_id ? (
                              <Spinner size="sm" />
                            ) : (
                              <RefreshCw className="h-3 w-3 text-red-600 dark:text-red-400" />
                            )}
                          </button>
                        </Tooltip>
                      )}
                    </div>
                  </TableCell>
                  <TableCell className="text-muted-foreground text-xs">
                    {rawRestartCount != null ? (
                      restartCount > 0 ? (
                        <Tooltip text={formatRestartReasons(restartDetails, restartCount)} placement="top">
                          <span className="cursor-help underline decoration-dotted">{restartCount + 1}</span>
                        </Tooltip>
                      ) : (
                        <span>{restartCount + 1}</span>
                      )
                    ) : '-'}
                  </TableCell>
                  <TableCell>
                    {ctxLeft != null ? (
                      row.status === 'running' && ctxLeft <= restartThreshold ? (
                        <Tooltip text={`Restart at ≤${restartThreshold}%`} placement="top">
                          <span className="flex items-center gap-0.5 cursor-help">
                            <AlertTriangle className="h-3 w-3 text-amber-500" />
                            <span className="text-xs text-amber-600 dark:text-amber-400">{ctxLeft}%</span>
                          </span>
                        </Tooltip>
                      ) : (
                        <span className={cn('text-xs', contextLeftColor(ctxLeft))}>{ctxLeft}%</span>
                      )
                    ) : (
                      <span className="text-muted-foreground text-xs">-</span>
                    )}
                  </TableCell>
                  <TableCell className="text-muted-foreground text-xs">{getDuration(row)}</TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>
      {confirmSessionId && confirmRow && (
        <ConfirmDialog
          open={true}
          onClose={() => setConfirmSessionId(null)}
          onConfirm={() => onRetryFailed!(confirmSessionId)}
          title="Retry Failed Agent"
          message={`This will retry the failed "${confirmRow.history?.agent_type ?? ''}" agent from the failed layer. All agents in this layer will be re-run.`}
          confirmLabel="Retry"
          variant="destructive"
        />
      )}
    </>
  )
}
