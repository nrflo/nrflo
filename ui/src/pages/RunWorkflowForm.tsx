import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Play, Sparkles } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { Spinner } from '@/components/ui/Spinner'
import { Tooltip } from '@/components/ui/Tooltip'
import { Textarea } from '@/components/ui/Textarea'
import { Toggle } from '@/components/ui/Toggle'
import { listAgentDefs } from '@/api/agentDefs'
import { getGlobalSettings, settingsKeys } from '@/api/settings'
import { ArtifactUploader } from '@/components/workflow/ArtifactUploader'
import { useStartDynamicWorkflow } from '@/hooks/usePlan'
import type { AgentDef, WorkflowDefSummary } from '@/types/workflow'
import type { InputArtifactRef } from '@/types/artifact'
import type { StartMode } from './ProjectWorkflowComponents'

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
  onStagedArtifactsChange,
  hasUploadPending,
  onUploadPendingChange,
  projectId,
  onDynamicRunSuccess,
}: {
  projectWorkflows: [string, { description: string; scope_type?: string; is_global?: boolean; phases: WorkflowDefSummary['phases'] }][]
  defsLoading: boolean
  selectedWorkflowDef: string
  onSelectWorkflowDef: (v: string) => void
  instructions: string
  onInstructionsChange: (v: string) => void
  onRun: (startMode: StartMode) => void
  runPending: boolean
  runError: Error | null
  onStagedArtifactsChange: (refs: InputArtifactRef[]) => void
  hasUploadPending: boolean
  onUploadPendingChange: (pending: boolean) => void
  projectId?: string
  onDynamicRunSuccess?: (instanceId: string) => void
}) {
  const [startMode, setStartMode] = useState<StartMode>('normal')
  const [dynamicInstructions, setDynamicInstructions] = useState('')
  const [dynamicAutoMode, setDynamicAutoMode] = useState(false)

  const { data: globalSettings } = useQuery({
    queryKey: settingsKeys.global(),
    queryFn: getGlobalSettings,
  })
  const startDynamicMutation = useStartDynamicWorkflow()
  const dynamicAutoAllowed = globalSettings?.dynamic_workflow_auto_enabled ?? false

  const handleStartDynamic = () => {
    const pid = projectId ?? ''
    if (!pid || !dynamicInstructions.trim()) return
    startDynamicMutation.mutate(
      {
        projectId: pid,
        params: {
          instructions: dynamicInstructions,
          mode: dynamicAutoMode && dynamicAutoAllowed ? 'auto' : 'approve',
        },
      },
      {
        onSuccess: (result) => {
          setDynamicInstructions('')
          setDynamicAutoMode(false)
          onDynamicRunSuccess?.(result.instance_id)
        },
      }
    )
  }

  const { data: agents } = useQuery({
    queryKey: ['workflows', selectedWorkflowDef, 'agents'],
    queryFn: () => listAgentDefs(selectedWorkflowDef),
    enabled: !!selectedWorkflowDef,
  })

  const selectedDef = projectWorkflows.find(([id]) => id === selectedWorkflowDef)?.[1]
  const isProjectScoped = selectedDef?.scope_type === 'project'

  const { canInteractive } = useMemo(() => {
    if (!selectedDef || !agents) return { canInteractive: false }

    const l0Phases = selectedDef.phases.filter((p) => p.layer === 0)
    if (l0Phases.length !== 1) return { canInteractive: false }

    const hasMultipleLayers = selectedDef.phases.some((p) => p.layer > 0)
    if (!hasMultipleLayers) return { canInteractive: false }

    const l0AgentId = l0Phases[0].agent
    const agentDef = agents.find((a: AgentDef) => a.id === l0AgentId)
    if (!agentDef) {
      return { canInteractive: false }
    }

    return { canInteractive: true }
  }, [selectedDef, agents])

  const canPlan = useMemo(() => {
    if (!selectedDef || !agents) return false

    const l0Phases = selectedDef.phases.filter((p) => p.layer === 0)
    if (l0Phases.length !== 1) return false
    return l0Phases.some((p) => agents.find((a: AgentDef) => a.id === p.agent))
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
            label: id + (def.is_global ? ' (global)' : '') + (def.description ? ` - ${def.description}` : ''),
          }))}
        />
      </div>

      {(canInteractive || canPlan || isProjectScoped) && (
        <div className="flex gap-4 flex-wrap">
          {canInteractive && (
            <Tooltip text="Launches only the first-layer agent in a live terminal session. You interact with the agent directly, then remaining layers run automatically after you exit." placement="top" className="whitespace-normal max-w-xs">
              <label className="flex items-center gap-2 text-sm cursor-pointer">
                <input
                  type="checkbox"
                  checked={startMode === 'interactive'}
                  onChange={(e) => setStartMode(e.target.checked ? 'interactive' : 'normal')}
                  disabled={startMode === 'endless'}
                  className="rounded border-input disabled:opacity-50"
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
                  disabled={startMode === 'endless'}
                  className="rounded border-input disabled:opacity-50"
                />
                Plan Before Execution
              </label>
            </Tooltip>
          )}
          {isProjectScoped && (
            <Tooltip text="Keep re-running this workflow after each successful completion. A failure terminates the loop. You can request a graceful stop from the running workflow view." placement="top" className="whitespace-normal max-w-xs">
              <label className="flex items-center gap-2 text-sm cursor-pointer">
                <input
                  type="checkbox"
                  checked={startMode === 'endless'}
                  onChange={(e) => {
                    if (e.target.checked) {
                      setStartMode('endless')
                      onInstructionsChange('')
                    } else {
                      setStartMode('normal')
                    }
                  }}
                  className="rounded border-input"
                />
                Endless loop
              </label>
            </Tooltip>
          )}
        </div>
      )}

      {startMode !== 'plan' && startMode !== 'endless' && (
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

      <div>
        <label className="block text-sm font-medium mb-1.5">
          Attachments <span className="text-muted-foreground font-normal">(optional)</span>
        </label>
        <ArtifactUploader
          onChange={(refs, pending) => {
            onStagedArtifactsChange(refs)
            onUploadPendingChange(pending)
          }}
        />
      </div>

      {runError && (
        <p className="text-sm text-destructive">
          {runError instanceof Error ? runError.message : 'Failed to start workflow'}
        </p>
      )}

      <Button
        onClick={() => onRun(startMode)}
        disabled={!selectedWorkflowDef || runPending || hasUploadPending}
      >
        {runPending && <Spinner size="sm" className="mr-2" />}
        <Play className="h-4 w-4 mr-2" />
        Run
      </Button>

      <div className="rounded-md border border-border p-4 space-y-3">
        <div>
          <h3 className="text-sm font-medium">Dynamic (planned) run</h3>
          <p className="text-xs text-muted-foreground mt-0.5">
            A planner agent drafts a multi-agent plan from your instructions; review and approve it before it runs.
          </p>
        </div>
        <Textarea
          value={dynamicInstructions}
          onChange={(e) => setDynamicInstructions(e.target.value)}
          placeholder="Describe the goal for the planner to turn into a plan..."
          rows={4}
        />
        <div className="flex items-center gap-2">
          <Toggle
            checked={dynamicAutoMode}
            onChange={setDynamicAutoMode}
            disabled={!dynamicAutoAllowed}
            label="Auto-approve plan"
          />
          {!dynamicAutoAllowed && (
            <span className="text-xs text-muted-foreground">
              Enable &quot;Allow dynamic_workflow mode=auto&quot; in Settings to skip manual review
            </span>
          )}
        </div>
        {startDynamicMutation.isError && (
          <p className="text-sm text-destructive">
            {startDynamicMutation.error instanceof Error ? startDynamicMutation.error.message : 'Failed to start dynamic run'}
          </p>
        )}
        <Button
          variant="outline"
          onClick={handleStartDynamic}
          disabled={!projectId || !dynamicInstructions.trim() || startDynamicMutation.isPending}
        >
          {startDynamicMutation.isPending && <Spinner size="sm" className="mr-2" />}
          <Sparkles className="h-4 w-4 mr-2" />
          Start Dynamic Run
        </Button>
      </div>
    </div>
  )
}
