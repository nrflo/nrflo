import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import cronstrue from 'cronstrue'
import { Check } from 'lucide-react'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Toggle } from '@/components/ui/Toggle'
import { listWorkflowDefs } from '@/api/workflows'
import { useCreateScheduledTask, useUpdateScheduledTask } from '@/hooks/useScheduledTasks'
import { useProjectStore } from '@/stores/projectStore'
import { cn, formatDateTime } from '@/lib/utils'
import { computeNextRuns, formatCountdown } from '@/lib/cron'
import { useTickingClock } from '@/hooks/useElapsedTime'
import type { ScheduledTask } from '@/types/schedules'

interface NextRunsPreviewProps {
  expression: string
}

function NextRunsPreview({ expression }: NextRunsPreviewProps) {
  useTickingClock(true)
  const now = new Date()
  const runs = computeNextRuns(expression, 5)
  if (runs.length === 0) return null
  return (
    <div className="text-xs text-muted-foreground space-y-0.5 mt-1">
      {runs.map((d, i) => (
        <p key={i}>
          {formatDateTime(d)} · {formatCountdown(d, now)}
        </p>
      ))}
    </div>
  )
}

interface ScheduleFormProps {
  open: boolean
  onClose: () => void
  editTarget?: ScheduledTask
}

export function ScheduleForm({ open, onClose, editTarget }: ScheduleFormProps) {
  const currentProject = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  const isEdit = !!editTarget

  const [name, setName] = useState(editTarget?.name ?? '')
  const [description, setDescription] = useState(editTarget?.description ?? '')
  const [cronExpression, setCronExpression] = useState(editTarget?.cron_expression ?? '')
  const [workflows, setWorkflows] = useState<string[]>(editTarget?.workflows ?? [])
  const [enabled, setEnabled] = useState(editTarget?.enabled ?? true)

  const { data: workflowDefs } = useQuery({
    queryKey: ['workflows', 'defs', currentProject],
    queryFn: listWorkflowDefs,
    enabled: projectsLoaded,
  })

  const projectWorkflows = workflowDefs
    ? Object.entries(workflowDefs).filter(([, def]) => def.scope_type === 'project')
    : []

  let cronDescription = ''
  let cronError = ''
  if (cronExpression.trim()) {
    try {
      cronDescription = cronstrue.toString(cronExpression, { throwExceptionOnParseError: true })
    } catch {
      cronError = 'Invalid cron expression'
    }
  }

  const canSubmit = name.trim() && cronExpression.trim() && !cronError && workflows.length > 0

  const createMutation = useCreateScheduledTask()
  const updateMutation = useUpdateScheduledTask()
  const isPending = createMutation.isPending || updateMutation.isPending

  const toggleWorkflow = (wf: string) => {
    setWorkflows((prev) =>
      prev.includes(wf) ? prev.filter((w) => w !== wf) : [...prev, wf]
    )
  }

  const handleSubmit = () => {
    if (!canSubmit) return
    if (isEdit && editTarget) {
      updateMutation.mutate(
        { id: editTarget.id, data: { name, description, cron_expression: cronExpression, workflows, enabled } },
        { onSuccess: onClose }
      )
    } else {
      createMutation.mutate(
        { name, description, cron_expression: cronExpression, workflows, enabled },
        { onSuccess: onClose }
      )
    }
  }

  return (
    <Dialog open={open} onClose={onClose}>
      <DialogHeader onClose={onClose}>
        <h2 className="text-lg font-semibold">{isEdit ? 'Edit Schedule' : 'New Schedule'}</h2>
      </DialogHeader>
      <DialogBody className="space-y-4">
        <div className="space-y-1.5">
          <label className="text-sm font-medium">Name</label>
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="My schedule"
          />
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium">Description</label>
          <Input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Optional description"
          />
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium">Cron Expression</label>
          <Input
            value={cronExpression}
            onChange={(e) => setCronExpression(e.target.value)}
            placeholder="0 9 * * 1-5"
            className={cronError ? 'border-destructive' : ''}
          />
          {cronError ? (
            <p className="text-xs text-destructive">{cronError}</p>
          ) : cronDescription ? (
            <>
              <p className="text-xs text-muted-foreground">{cronDescription}</p>
              <NextRunsPreview expression={cronExpression} />
            </>
          ) : null}
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium">Project Workflows</label>
          {projectWorkflows.length === 0 ? (
            <p className="text-sm text-muted-foreground border border-border rounded-lg p-3">
              No project workflows found
            </p>
          ) : (
            <div className="border border-border rounded-lg p-2 space-y-1 max-h-40 overflow-y-auto">
              {projectWorkflows.map(([wfName]) => {
                const selected = workflows.includes(wfName)
                return (
                  <button
                    key={wfName}
                    type="button"
                    onClick={() => toggleWorkflow(wfName)}
                    className={cn(
                      'flex items-center gap-2 w-full px-2 py-1.5 rounded text-sm text-left transition-colors',
                      selected ? 'bg-primary/10 text-primary' : 'hover:bg-muted'
                    )}
                  >
                    <div className={cn(
                      'h-4 w-4 rounded border flex items-center justify-center shrink-0',
                      selected ? 'bg-primary border-primary' : 'border-muted-foreground/40'
                    )}>
                      {selected && <Check className="h-3 w-3 text-primary-foreground" />}
                    </div>
                    {wfName}
                  </button>
                )
              })}
            </div>
          )}
          {workflows.length === 0 && (
            <p className="text-xs text-muted-foreground">Select at least one workflow</p>
          )}
        </div>
        <div className="flex items-center gap-3">
          <Toggle checked={enabled} onChange={setEnabled} label="Enabled" />
        </div>
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" onClick={onClose} disabled={isPending}>
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={!canSubmit || isPending}>
          {isPending ? 'Saving…' : isEdit ? 'Save Changes' : 'Create'}
        </Button>
      </DialogFooter>
    </Dialog>
  )
}
