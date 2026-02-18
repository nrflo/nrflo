import { Play } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Dropdown } from '@/components/ui/Dropdown'
import { Spinner } from '@/components/ui/Spinner'
import { cn, formatElapsedTime } from '@/lib/utils'
import type { WorkflowState } from '@/types/workflow'

// --- Inline Run Workflow Form ---

export function RunWorkflowForm({
  projectWorkflows,
  defsLoading,
  selectedWorkflowDef,
  onSelectWorkflowDef,
  instructions,
  onInstructionsChange,
  onRun,
  runPending,
  runError,
}: {
  projectWorkflows: [string, { description: string; scope_type?: string }][]
  defsLoading: boolean
  selectedWorkflowDef: string
  onSelectWorkflowDef: (v: string) => void
  instructions: string
  onInstructionsChange: (v: string) => void
  onRun: () => void
  runPending: boolean
  runError: Error | null
}) {
  if (defsLoading) {
    return (
      <div className="flex justify-center py-8">
        <Spinner />
      </div>
    )
  }

  if (projectWorkflows.length === 0) {
    return (
      <p className="text-muted-foreground text-sm text-center py-8">
        No project-scoped workflow definitions found. Create one with scope &quot;project&quot; on the Workflows page.
      </p>
    )
  }

  return (
    <div className="max-w-xl space-y-4">
      <div>
        <label htmlFor="project-workflow-select" className="block text-sm font-medium mb-1.5">Workflow</label>
        <Dropdown
          value={selectedWorkflowDef}
          onChange={onSelectWorkflowDef}
          options={projectWorkflows.map(([id, def]) => ({
            value: id,
            label: id + (def.description ? ` - ${def.description}` : ''),
          }))}
        />
      </div>

      <div>
        <label className="block text-sm font-medium mb-1.5">
          Instructions <span className="text-muted-foreground font-normal">(optional)</span>
        </label>
        <textarea
          value={instructions}
          onChange={(e) => onInstructionsChange(e.target.value)}
          placeholder="Additional context or instructions for the agents..."
          rows={4}
          className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring resize-none"
        />
      </div>

      {runError && (
        <p className="text-sm text-destructive">
          {runError instanceof Error ? runError.message : 'Failed to start workflow'}
        </p>
      )}

      <Button
        onClick={onRun}
        disabled={!selectedWorkflowDef || runPending}
      >
        {runPending && <Spinner size="sm" className="mr-2" />}
        <Play className="h-4 w-4 mr-2" />
        Run
      </Button>
    </div>
  )
}

// --- Instance List ---

export function InstanceList({
  instanceIds,
  instances,
  labels,
  selectedId,
  onSelect,
  tab,
}: {
  instanceIds: string[]
  instances: Record<string, WorkflowState>
  labels: Record<string, string>
  selectedId: string
  onSelect: (id: string) => void
  tab: 'running' | 'completed'
}) {
  return (
    <div className="flex flex-wrap gap-2">
      {instanceIds.map((id) => {
        const state = instances[id]
        const isSelected = id === selectedId
        return (
          <button
            key={id}
            onClick={() => onSelect(id)}
            className={cn(
              'flex items-center gap-2 px-3 py-1.5 rounded-md border text-sm transition-colors',
              isSelected
                ? 'border-primary bg-primary/10 text-primary'
                : 'border-border hover:border-primary/50 text-foreground'
            )}
          >
            <span className="font-medium">{labels[id] ?? id}</span>
            {tab === 'running' && (
              <Badge
                variant={state?.status === 'failed' ? 'destructive' : 'default'}
                className="text-xs"
              >
                {state?.status ?? 'active'}
              </Badge>
            )}
            {tab === 'completed' && (
              <Badge variant="success" className="text-xs">completed</Badge>
            )}
            {state?.current_phase && tab === 'running' && (
              <span className="text-xs text-muted-foreground">{state.current_phase}</span>
            )}
            {state?.completed_at && tab === 'running' && state?.status !== 'completed' && (
              <span className="text-xs text-muted-foreground">
                {formatElapsedTime(state.completed_at)}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}
