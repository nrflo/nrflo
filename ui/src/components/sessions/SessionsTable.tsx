import { Badge } from '@/components/ui/Badge'
import { StatusCell } from '@/components/ui/StatusCell'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { formatCost } from '@/lib/systemAgentRuns'
import { formatDateTime, formatElapsedTime } from '@/lib/utils'
import type { SessionListRow } from '@/types/session'

export function SessionsTable({
  sessions,
  selectedId,
  onSelect,
}: {
  sessions: SessionListRow[]
  selectedId?: string
  onSelect: (sessionId: string) => void
}) {
  if (sessions.length === 0) {
    return <div className="text-center text-muted-foreground py-12">No sessions</div>
  }

  return (
    <Table>
      <TableHeader>
        <TableRow className="bg-muted/30">
          <TableHead className="w-24">SID</TableHead>
          <TableHead className="w-24">Kind</TableHead>
          <TableHead className="w-28">Agent</TableHead>
          <TableHead className="w-32">Model</TableHead>
          <TableHead className="w-28">Status</TableHead>
          <TableHead className="w-20">Cost</TableHead>
          <TableHead className="w-40">Started</TableHead>
          <TableHead className="w-24">Duration</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {sessions.map((row) => (
          <TableRow
            key={row.session_id}
            data-state={selectedId === row.session_id ? 'selected' : undefined}
            className="font-mono text-xs cursor-pointer"
            onClick={() => onSelect(row.session_id)}
          >
            <TableCell title={row.session_id}>{row.session_id.substring(0, 8)}</TableCell>
            <TableCell>
              <Badge variant="outline">{row.kind}</Badge>
            </TableCell>
            <TableCell className="text-muted-foreground">{row.agent_type ?? '—'}</TableCell>
            <TableCell className="text-muted-foreground">{row.model_id ?? '—'}</TableCell>
            <TableCell>
              <StatusCell status={row.status} />
            </TableCell>
            <TableCell className="text-muted-foreground">{formatCost(row.cost_estimate)}</TableCell>
            <TableCell className="text-muted-foreground">
              {row.started_at ? formatDateTime(row.started_at) : '—'}
            </TableCell>
            <TableCell className="text-muted-foreground">
              {row.started_at && row.ended_at ? formatElapsedTime(row.started_at, row.ended_at) : '—'}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
