import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CheckCircle, Play, XCircle } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Dropdown } from '@/components/ui/Dropdown'
import { Spinner } from '@/components/ui/Spinner'
import { Tooltip } from '@/components/ui/Tooltip'
import { listAgentDefs } from '@/api/agentDefs'
import { cn, formatElapsedTime } from '@/lib/utils'
import type { WorkflowState, AgentDef, WorkflowDefSummary } from '@/types/workflow'

type StartMode = 'normal' | 'interactive' | 'plan'

function isClaudeModel(model: string): boolean {
  return !model.startsWith('opencode_') && !model.startsWith('codex_gpt_')
}

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
  projectWorkflows: [string, { description: string; scope_type?: string; phases: WorkflowDefSummary['phases'] }][]
  defsLoading: boolean
  selectedWorkflowDef: string
  onSelectWorkflowDef: (v: string) => void
  instructions: string
  onInstructionsChange: (v: string) => void
  onRun: (startMode: StartMode) => void
  runPending: boolean
  runError: Error | null
}) {
  const [startMode, setStartMode] = useState<StartMode>('normal')

  const { data: agents } = useQuery({
    queryKey: ['workflows', selectedWorkflowDef, 'agents'],
    queryFn: () => listAgentDefs(selectedWorkflowDef),
    enabled: !!selectedWorkflowDef,
  })

  const selectedDef = projectWorkflows.find(([id]) => id === selectedWorkflowDef)?.[1]

  const { canInteractive } = useMemo(() => {
    if (!selectedDef || !agents) return { canInteractive: false }

    const l0Phases = selectedDef.phases.filter((p) => p.layer === 0)
    if (l0Phases.length !== 1) return { canInteractive: false }

    const hasMultipleLayers = selectedDef.phases.some((p) => p.layer > 0)
    if (!hasMultipleLayers) return { canInteractive: false }

    const l0AgentId = l0Phases[0].agent
    const agentDef = agents.find((a: AgentDef) => a.id === l0AgentId)
    if (!agentDef || !isClaudeModel(agentDef.model)) {
      return { canInteractive: false }
    }

    return { canInteractive: true }
  }, [selectedDef, agents])

  const canPlan = useMemo(() => {
    if (!selectedDef || !agents) return false

    const l0Phases = selectedDef.phases.filter((p) => p.layer === 0)
    if (l0Phases.length !== 1) return false
    return l0Phases.some((p) => {
      const agentDef = agents.find((a: AgentDef) => a.id === p.agent)
      return agentDef && isClaudeModel(agentDef.model)
    })
  }, [selectedDef, agents])

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
    <div className="max-w-3xl space-y-4">
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

      {(canInteractive || canPlan) && (
        <div className="flex gap-4">
          {canInteractive && (
            <Tooltip text="Launches only the first-layer agent in a live terminal session. You interact with the agent directly, then remaining layers run automatically after you exit." placement="top" className="whitespace-normal max-w-xs">
              <label className="flex items-center gap-2 text-sm cursor-pointer">
                <input
                  type="checkbox"
                  checked={startMode === 'interactive'}
                  onChange={(e) => setStartMode(e.target.checked ? 'interactive' : 'normal')}
                  className="rounded border-input"
                />
                Start Interactive
              </label>
            </Tooltip>
          )}
          {canPlan && (
            <Tooltip text="Spawns a planner agent in a live terminal. Collaborate with the planner to define the approach — the resulting plan is used as User Instructions for all downstream agents. Then the full workflow executes automatically." placement="top" className="whitespace-normal max-w-xs">
              <label className="flex items-center gap-2 text-sm cursor-pointer">
                <input
                  type="checkbox"
                  checked={startMode === 'plan'}
                  onChange={(e) => setStartMode(e.target.checked ? 'plan' : 'normal')}
                  className="rounded border-input"
                />
                Plan Before Execution
              </label>
            </Tooltip>
          )}
        </div>
      )}

      {startMode !== 'plan' && (
        <div>
          <label className="block text-sm font-medium mb-1.5">
            Instructions <span className="text-muted-foreground font-normal">(optional)</span>
          </label>
          <textarea
            value={instructions}
            onChange={(e) => onInstructionsChange(e.target.value)}
            placeholder="Additional context or instructions for the agents..."
            rows={12}
            className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring resize-y"
          />
        </div>
      )}

      {runError && (
        <p className="text-sm text-destructive">
          {runError instanceof Error ? runError.message : 'Failed to start workflow'}
        </p>
      )}

      <Button
        onClick={() => onRun(startMode)}
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

// --- Tab Bar ---

export type ProjectWorkflowTabId = 'run' | 'running' | 'failed' | 'completed'

export function ProjectWorkflowTabBar({
  activeTab,
  onTabSwitch,
  runningCount,
  failedCount,
  completedCount,
}: {
  activeTab: ProjectWorkflowTabId
  onTabSwitch: (tab: ProjectWorkflowTabId) => void
  runningCount: number
  failedCount: number
  completedCount: number
}) {
  const tabs: { id: ProjectWorkflowTabId; label: string; icon?: typeof Play; count?: number }[] = [
    { id: 'run', label: 'Run Workflow', icon: Play },
    { id: 'running', label: 'Running', count: runningCount },
    { id: 'failed', label: 'Failed', icon: XCircle, count: failedCount },
    { id: 'completed', label: 'Completed', icon: CheckCircle, count: completedCount },
  ]

  return (
    <div className="border-b border-border">
      <div className="flex gap-1">
        {tabs.map(({ id, label, icon: Icon, count }) => (
          <button
            key={id}
            onClick={() => onTabSwitch(id)}
            className={cn(
              'flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors',
              activeTab === id
                ? 'border-primary text-primary'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            )}
          >
            {Icon && <Icon className="h-4 w-4" />}
            {count !== undefined ? `${label} (${count})` : label}
          </button>
        ))}
      </div>
    </div>
  )
}
