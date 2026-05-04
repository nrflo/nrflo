import { useState } from 'react'
import cronstrue from 'cronstrue'
import { Plus, Pencil, Trash2, Play, List } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import { Toggle } from '@/components/ui/Toggle'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { ReadOnlyHint } from '@/components/auth/ReadOnlyHint'
import { ScheduleForm } from './ScheduleForm'
import { ScheduleRunsDialog } from './ScheduleRunsDialog'
import { useScheduledTasks, useDeleteScheduledTask, useUpdateScheduledTask, useRunScheduleNow } from '@/hooks/useScheduledTasks'
import { formatRelativeTime, cn } from '@/lib/utils'
import { CronCountdown } from '@/components/ui/CronCountdown'
import { useIsAdmin } from '@/stores/authStore'
import type { ScheduledTask } from '@/types/schedules'

function cronSummary(expr: string): string {
  try {
    return cronstrue.toString(expr, { throwExceptionOnParseError: true })
  } catch {
    return ''
  }
}

export function SchedulesPage() {
  const [showCreate, setShowCreate] = useState(false)
  const [editTarget, setEditTarget] = useState<ScheduledTask | null>(null)
  const [deleteTargetId, setDeleteTargetId] = useState<string | null>(null)
  const [runsTarget, setRunsTarget] = useState<ScheduledTask | null>(null)
  const isAdmin = useIsAdmin()

  const { data: tasks, isLoading, error } = useScheduledTasks()
  const deleteMutation = useDeleteScheduledTask()
  const updateMutation = useUpdateScheduledTask()
  const runNowMutation = useRunScheduleNow()

  const handleToggleEnabled = (task: ScheduledTask) => {
    updateMutation.mutate({ id: task.id, data: { enabled: !task.enabled } })
  }

  const handleRunNow = (id: string) => {
    runNowMutation.mutate(id)
  }

  return (
    <div className="max-w-[85%] mx-auto space-y-6">
      {!isAdmin && <ReadOnlyHint />}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Schedules</h1>
          <p className="text-muted-foreground">
            {tasks?.length ?? 0} task{tasks?.length !== 1 ? 's' : ''}
          </p>
        </div>
        {isAdmin && (
          <Button onClick={() => setShowCreate(true)}>
            <Plus className="h-4 w-4 mr-2" />
            New Schedule
          </Button>
        )}
      </div>

      {isLoading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : error ? (
        <p className="text-destructive text-sm">
          {error instanceof Error ? error.message : 'Failed to load schedules'}
        </p>
      ) : !tasks || tasks.length === 0 ? (
        <div className="text-center py-12 text-muted-foreground">
          <p>No schedules found. Create one to get started!</p>
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead className="w-64">Cron Expression</TableHead>
              <TableHead className="w-36">Triggers</TableHead>
              <TableHead className="w-32">Last Run</TableHead>
              <TableHead className="w-32">Next Run</TableHead>
              <TableHead className="w-20">Enabled</TableHead>
              <TableHead className="w-32" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {tasks.map((task) => {
              const summary = cronSummary(task.cron_expression)
              return (
                <TableRow key={task.id}>
                  <TableCell>
                    <div>
                      <span className="font-medium">{task.name}</span>
                      {task.description && (
                        <p className="text-xs text-muted-foreground mt-0.5">{task.description}</p>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>
                    <div>
                      <code className="text-xs bg-muted px-1.5 py-0.5 rounded">{task.cron_expression}</code>
                      {summary && (
                        <p className="text-xs text-muted-foreground mt-0.5">{summary}</p>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1 flex-wrap">
                      {task.workflows.length > 0 && (
                        <Badge variant="secondary" title={`${task.workflows.length} workflow${task.workflows.length !== 1 ? 's' : ''}`}>
                          {task.workflows.length} wf
                        </Badge>
                      )}
                      {task.workflow_chain_ids?.length > 0 && (
                        <Badge variant="secondary" title={`${task.workflow_chain_ids.length} chain${task.workflow_chain_ids.length !== 1 ? 's' : ''}`}>
                          {task.workflow_chain_ids.length} ch
                        </Badge>
                      )}
                      {task.workflows.length === 0 && !task.workflow_chain_ids?.length && (
                        <span className="text-muted-foreground text-sm">—</span>
                      )}
                    </div>
                  </TableCell>
                  <TableCell className="text-muted-foreground text-sm">
                    {task.last_triggered_at ? formatRelativeTime(task.last_triggered_at) : '—'}
                  </TableCell>
                  <TableCell>
                    <CronCountdown nextRunAt={task.next_run_at} />
                  </TableCell>
                  <TableCell>
                    <Toggle
                      checked={task.enabled}
                      onChange={() => isAdmin && handleToggleEnabled(task)}
                      disabled={!isAdmin || updateMutation.isPending}
                    />
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <button
                        onClick={() => setRunsTarget(task)}
                        className="p-1 text-muted-foreground hover:text-foreground transition-colors"
                        title="View runs"
                      >
                        <List className="h-3.5 w-3.5" />
                      </button>
                      {isAdmin && (
                        <>
                          <button
                            onClick={() => handleRunNow(task.id)}
                            disabled={runNowMutation.isPending}
                            className={cn(
                              'p-1 text-muted-foreground hover:text-foreground transition-colors',
                              runNowMutation.isPending && 'opacity-50 cursor-not-allowed'
                            )}
                            title="Run now"
                          >
                            <Play className="h-3.5 w-3.5" />
                          </button>
                          <button
                            onClick={() => setEditTarget(task)}
                            className="p-1 text-muted-foreground hover:text-foreground transition-colors"
                            title="Edit"
                          >
                            <Pencil className="h-3.5 w-3.5" />
                          </button>
                          <button
                            onClick={() => setDeleteTargetId(task.id)}
                            className="p-1 text-muted-foreground hover:text-destructive transition-colors"
                            title="Delete"
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </button>
                        </>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      )}

      {isAdmin && (
        <ScheduleForm
          open={showCreate}
          onClose={() => setShowCreate(false)}
        />
      )}
      {isAdmin && editTarget && (
        <ScheduleForm
          open={!!editTarget}
          onClose={() => setEditTarget(null)}
          editTarget={editTarget}
        />
      )}
      {runsTarget && (
        <ScheduleRunsDialog
          open={!!runsTarget}
          onClose={() => setRunsTarget(null)}
          task={runsTarget}
        />
      )}
      {isAdmin && (
        <ConfirmDialog
          open={!!deleteTargetId}
          onClose={() => setDeleteTargetId(null)}
          onConfirm={() => {
            if (deleteTargetId) {
              deleteMutation.mutate(deleteTargetId, {
                onSettled: () => setDeleteTargetId(null),
              })
            }
          }}
          title="Delete Schedule"
          message="Are you sure you want to delete this schedule? This action cannot be undone."
          confirmLabel="Delete"
          variant="destructive"
        />
      )}
    </div>
  )
}
