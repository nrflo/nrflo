import { useState } from 'react'
import { RefreshCw, Skull } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { formatMB, formatDurationSec } from '@/lib/utils'
import { useLiveAgentSessions, useKillAgentSession } from '@/hooks/useAgentSessionLogs'

export function LogsLiveTab() {
  const query = useLiveAgentSessions()
  const killMutation = useKillAgentSession()
  const [killTarget, setKillTarget] = useState<string | null>(null)

  const sessions = query.data?.sessions ?? []

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={() => query.refetch()}
          className="flex items-center gap-2"
        >
          <RefreshCw className="h-4 w-4" />
          {query.isFetching ? 'Loading…' : 'Refresh'}
        </Button>
      </div>

      {sessions.length === 0 ? (
        <div className="text-center text-muted-foreground py-12">
          No live processes
        </div>
      ) : (
        <div className="border rounded-lg overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/30">
                <TableHead className="w-24">SID</TableHead>
                <TableHead className="w-32">Agent</TableHead>
                <TableHead className="w-32">Model</TableHead>
                <TableHead className="w-20">Mode</TableHead>
                <TableHead className="w-40">Workflow</TableHead>
                <TableHead className="w-24">Uptime</TableHead>
                <TableHead className="w-20">PID</TableHead>
                <TableHead className="w-24">Memory</TableHead>
                <TableHead className="w-20">CPU %</TableHead>
                <TableHead className="w-20">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sessions.map((session) => (
                <TableRow key={session.session_id} className="font-mono text-xs">
                  <TableCell>
                    <span title={session.session_id} className="font-mono">
                      {session.session_id.substring(0, 8)}
                    </span>
                  </TableCell>
                  <TableCell>{session.agent_type}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {session.model_id ?? <span>{'—'}</span>}
                  </TableCell>
                  <TableCell>
                    {session.execution_mode ? (
                      <Badge>{session.execution_mode}</Badge>
                    ) : (
                      <span className="text-muted-foreground">{'—'}</span>
                    )}
                  </TableCell>
                  <TableCell>{session.workflow_id}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDurationSec(session.os_uptime_sec)}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {session.pid}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatMB(session.rss_kb / 1024)}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {session.cpu_pct.toFixed(1)}%
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="destructive"
                      size="sm"
                      disabled={killMutation.isPending}
                      onClick={() => setKillTarget(session.session_id)}
                    >
                      <Skull className="h-4 w-4" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <ConfirmDialog
        open={killTarget !== null}
        onClose={() => setKillTarget(null)}
        onConfirm={() => {
          if (killTarget) killMutation.mutate(killTarget)
        }}
        title="Kill agent session"
        message="Force-kill this agent? Status becomes failed."
        confirmLabel="Kill"
        variant="destructive"
      />
    </div>
  )
}
