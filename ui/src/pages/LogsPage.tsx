import { useState } from 'react'
import { ChevronLeft, ChevronRight, CalendarClock, CheckCircle2 } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { Tooltip } from '@/components/ui/Tooltip'
import { StatusCell } from '@/components/ui/StatusCell'
import { formatDateTime, formatElapsedTime, formatDurationSec } from '@/lib/utils'
import { useAgentSessionLogs } from '@/hooks/useAgentSessionLogs'

const PAGE_SIZE = 20

type BadgeVariant = 'default' | 'secondary' | 'destructive' | 'outline' | 'success'
const executionModeMap: Record<string, { label: string; variant: BadgeVariant }> = {
  cli_interactive: { label: 'CLI interactive', variant: 'outline' },
}

export function LogsPage() {
  const [page, setPage] = useState(1)

  const { data, isLoading } = useAgentSessionLogs({ page, perPage: PAGE_SIZE })

  const sessions = data?.sessions ?? []
  const total = data?.total ?? 0
  const totalPages = data?.total_pages ?? 1

  const startItem = (page - 1) * PAGE_SIZE + 1
  const endItem = Math.min(page * PAGE_SIZE, total)

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-bold">Logs</h1>

      {isLoading ? (
        <div className="text-sm text-muted-foreground">Loading...</div>
      ) : sessions.length === 0 ? (
        <div className="text-center text-muted-foreground py-12">
          No agent sessions yet
        </div>
      ) : (
        <div className="border rounded-lg overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/30">
                <TableHead className="w-40">Finished</TableHead>
                <TableHead className="w-24">SID</TableHead>
                <TableHead className="w-32">Agent</TableHead>
                <TableHead className="w-32">Model</TableHead>
                <TableHead className="w-20">Mode</TableHead>
                <TableHead className="w-40">Workflow</TableHead>
                <TableHead className="w-24">Duration</TableHead>
                <TableHead className="w-28">Status</TableHead>
                <TableHead className="w-16">Result</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sessions.map((session) => (
                <TableRow key={session.session_id} className="font-mono text-xs">
                  <TableCell className="text-muted-foreground">
                    {session.ended_at ? formatDateTime(session.ended_at) : <span className="text-muted-foreground">{'—'}</span>}
                  </TableCell>
                  <TableCell title={session.session_id}>
                    {session.session_id.substring(0, 8)}
                  </TableCell>
                  <TableCell>{session.agent_type}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {session.model_id ?? <span>{'—'}</span>}
                  </TableCell>
                  <TableCell>
                    {session.execution_mode ? (() => {
                      const m = executionModeMap[session.execution_mode]
                      return m
                        ? <Badge variant={m.variant}>{m.label}</Badge>
                        : <Badge>{session.execution_mode}</Badge>
                    })() : (
                      <span className="text-muted-foreground">{'—'}</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <span className="inline-flex items-center gap-1">
                      {session.workflow_id}
                      {session.scheduled && (
                        <Tooltip text="Triggered by scheduler">
                          <CalendarClock className="h-3 w-3 text-muted-foreground" />
                        </Tooltip>
                      )}
                    </span>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {session.started_at && session.ended_at
                      ? formatElapsedTime(session.started_at, session.ended_at)
                      : session.duration_sec != null
                        ? formatDurationSec(session.duration_sec)
                        : <span>{'—'}</span>}
                  </TableCell>
                  <TableCell>
                    <StatusCell status={session.status} />
                  </TableCell>
                  <TableCell>
                    {session.workflow_final_result ? (
                      <Tooltip text={session.workflow_final_result}>
                        <CheckCircle2 className="h-4 w-4 text-green-500" />
                      </Tooltip>
                    ) : (
                      <span className="text-muted-foreground">{'—'}</span>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>

          {totalPages > 1 && (
            <div className="flex items-center justify-between px-4 py-3 text-xs text-muted-foreground border-t">
              <span>
                {startItem}–{endItem} of {total}
              </span>
              <div className="flex gap-1">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page <= 1}
                  onClick={() => setPage((p) => p - 1)}
                  className="h-7 w-7 p-0"
                >
                  <ChevronLeft className="h-4 w-4" />
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page >= totalPages}
                  onClick={() => setPage((p) => p + 1)}
                  className="h-7 w-7 p-0"
                >
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
