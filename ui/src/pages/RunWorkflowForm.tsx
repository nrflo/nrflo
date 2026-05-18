import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Play } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { Spinner } from '@/components/ui/Spinner'
import { Tooltip } from '@/components/ui/Tooltip'
import { listAgentDefs } from '@/api/agentDefs'
import { ArtifactUploader } from '@/components/workflow/ArtifactUploader'
import type { AgentDef, WorkflowDefSummary } from '@/types/workflow'
import type { InputArtifactRef } from '@/types/artifact'
import type { StartMode } from './ProjectWorkflowComponents'

function isClaudeModel(model: string): boolean {
  return !model.startsWith('opencode_') && !model.startsWith('codex_gpt_')
}

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
  onStagedArtifactsChange: (refs: InputArtifactRef[]) => void
  hasUploadPending: boolean
  onUploadPendingChange: (pending: boolean) => void
}) {
  const [startMode, setStartMode] = useState<StartMode>('normal')

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
    </div>
  )
}
