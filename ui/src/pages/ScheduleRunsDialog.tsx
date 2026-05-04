import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronLeft, ChevronRight, ExternalLink } from 'lucide-react'
import { Dialog, DialogHeader, DialogBody } from '@/components/ui/Dialog'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { useScheduleRuns } from '@/hooks/useScheduledTasks'
import { statusColor, capitalize, formatRelativeTime } from '@/lib/utils'
import type { ScheduledTask } from '@/types/schedules'

interface ScheduleRunsDialogProps {
  open: boolean
  onClose: () => void
  task: ScheduledTask
}

export function ScheduleRunsDialog({ open, onClose, task }: ScheduleRunsDialogProps) {
  const [page, setPage] = useState(0)
  const { data: runs, isLoading } = useScheduleRuns(task.id, page)

  const hasMore = (runs?.length ?? 0) === 20
  const hasPrev = page > 0

  return (
    <Dialog open={open} onClose={onClose} className="max-w-3xl">
      <DialogHeader onClose={onClose}>
        <div>
          <h2 className="text-lg font-semibold">Run History</h2>
          <p className="text-sm text-muted-foreground">{task.name}</p>
        </div>
      </DialogHeader>
      <DialogBody>
        {isLoading ? (
          <div className="flex justify-center py-8">
            <Spinner size="lg" />
          </div>
        ) : !runs || runs.length === 0 ? (
          <p className="text-center py-8 text-muted-foreground text-sm">No runs yet</p>
        ) : (
          <div className="space-y-3">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-24">Status</TableHead>
                  <TableHead className="w-36">Triggered</TableHead>
                  <TableHead>Workflows</TableHead>
                  <TableHead>Chains</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {runs.map((run) => (
                  <TableRow key={run.id}>
                    <TableCell>
                      <Badge className={statusColor(run.status)}>
                        {capitalize(run.status)}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {formatRelativeTime(run.triggered_at)}
                    </TableCell>
                    <TableCell>
                      {run.workflows.length === 0 ? (
                        <span className="text-muted-foreground text-sm">—</span>
                      ) : (
                        <div className="space-y-1">
                          {run.workflows.map((wf, i) => (
                            <div key={i} className="flex items-center gap-2 text-sm">
                              <span className="text-muted-foreground">{wf.workflow}</span>
                              {wf.instance_id && (
                                <Link
                                  to={`/project-workflows?instance_id=${wf.instance_id}`}
                                  onClick={(e) => e.stopPropagation()}
                                  className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
                                >
                                  <ExternalLink className="h-3 w-3" />
                                  view
                                </Link>
                              )}
                              {wf.error && (
                                <span className="text-xs text-destructive">{wf.error}</span>
                              )}
                            </div>
                          ))}
                        </div>
                      )}
                      {run.error && (
                        <p className="text-xs text-destructive mt-1">{run.error}</p>
                      )}
                    </TableCell>
                    <TableCell>
                      {!run.chain_runs || run.chain_runs.length === 0 ? (
                        <span className="text-muted-foreground text-sm">—</span>
                      ) : (
                        <div className="space-y-1">
                          {run.chain_runs.map((cr, i) => (
                            <div key={i} className="flex items-center gap-2 text-sm">
                              <span className="text-muted-foreground font-mono text-xs">
                                {cr.chain_id.slice(0, 8)}
                              </span>
                              {cr.chain_run_id && (
                                <Link
                                  to={`/workflow-chains/${cr.chain_id}`}
                                  onClick={(e) => e.stopPropagation()}
                                  className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
                                >
                                  <ExternalLink className="h-3 w-3" />
                                  view
                                </Link>
                              )}
                              {cr.error && (
                                <span className="text-xs text-destructive">{cr.error}</span>
                              )}
                            </div>
                          ))}
                        </div>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {(hasPrev || hasMore) && (
              <div className="flex items-center justify-end gap-1 pt-1">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!hasPrev}
                  onClick={() => setPage((p) => p - 1)}
                  className="h-7 w-7 p-0"
                >
                  <ChevronLeft className="h-4 w-4" />
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!hasMore}
                  onClick={() => setPage((p) => p + 1)}
                  className="h-7 w-7 p-0"
                >
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            )}
          </div>
        )}
      </DialogBody>
    </Dialog>
  )
}
