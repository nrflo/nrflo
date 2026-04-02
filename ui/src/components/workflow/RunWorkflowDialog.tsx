import { useState, useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, Play } from 'lucide-react'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { Spinner } from '@/components/ui/Spinner'
import { Tooltip } from '@/components/ui/Tooltip'
import { ApiError } from '@/api/client'
import { listWorkflowDefs } from '@/api/workflows'
import { listAgentDefs } from '@/api/agentDefs'
import { useRunWorkflow } from '@/hooks/useTickets'
import { useProjectStore } from '@/stores/projectStore'
import type { AgentDef } from '@/types/workflow'

type StartMode = 'normal' | 'interactive' | 'plan'

function isClaudeModel(model: string): boolean {
  return !model.startsWith('opencode_gpt_') && !model.startsWith('codex_gpt_')
}

interface RunWorkflowDialogProps {
  open: boolean
  onClose: () => void
  ticketId: string
  onInteractiveStart?: (sessionId: string, agentType: string) => void
  blockedReason?: string
}

export function RunWorkflowDialog({ open, onClose, ticketId, onInteractiveStart, blockedReason }: RunWorkflowDialogProps) {
  const [selectedWorkflow, setSelectedWorkflow] = useState('')
  const [instructions, setInstructions] = useState('')
  const [startMode, setStartMode] = useState<StartMode>('normal')
  const [showConcurrentWarning, setShowConcurrentWarning] = useState(false)

  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)

  const { data: workflowDefs, isLoading } = useQuery({
    queryKey: ['workflows', 'defs', project],
    queryFn: listWorkflowDefs,
    enabled: open && projectsLoaded,
  })

  const { data: agents } = useQuery({
    queryKey: ['workflows', selectedWorkflow, 'agents'],
    queryFn: () => listAgentDefs(selectedWorkflow),
    enabled: !!selectedWorkflow,
  })

  const runMutation = useRunWorkflow()

  const workflowIds = workflowDefs ? Object.keys(workflowDefs) : []

  // Auto-select first workflow
  useEffect(() => {
    if (workflowIds.length > 0 && !selectedWorkflow) {
      setSelectedWorkflow(workflowIds[0])
    }
  }, [workflowIds, selectedWorkflow])

  // Compute canInteractive: L0 has exactly 1 agent AND that agent is Claude-based
  const { canInteractive, l0AgentType } = useMemo(() => {
    if (!workflowDefs || !selectedWorkflow || !agents) {
      return { canInteractive: false, l0AgentType: '' }
    }
    const def = workflowDefs[selectedWorkflow]
    if (!def) return { canInteractive: false, l0AgentType: '' }

    const l0Phases = def.phases.filter((p) => p.layer === 0)
    if (l0Phases.length !== 1) return { canInteractive: false, l0AgentType: '' }

    const hasMultipleLayers = def.phases.some((p) => p.layer > 0)
    if (!hasMultipleLayers) return { canInteractive: false, l0AgentType: '' }

    const l0AgentId = l0Phases[0].agent
    const agentDef = agents.find((a: AgentDef) => a.id === l0AgentId)
    if (!agentDef || !isClaudeModel(agentDef.model)) {
      return { canInteractive: false, l0AgentType: '' }
    }

    return { canInteractive: true, l0AgentType: l0AgentId }
  }, [workflowDefs, selectedWorkflow, agents])

  // Check if any L0 agent is Claude-based (for plan mode)
  const canPlan = useMemo(() => {
    if (!workflowDefs || !selectedWorkflow || !agents) return false
    const def = workflowDefs[selectedWorkflow]
    if (!def) return false

    const l0Phases = def.phases.filter((p) => p.layer === 0)
    if (l0Phases.length !== 1) return false
    return l0Phases.some((p) => {
      const agentDef = agents.find((a: AgentDef) => a.id === p.agent)
      return agentDef && isClaudeModel(agentDef.model)
    })
  }, [workflowDefs, selectedWorkflow, agents])

  const handleRun = async () => {
    if (!selectedWorkflow) return
    try {
      const result = await runMutation.mutateAsync({
        ticketId,
        params: {
          workflow: selectedWorkflow,
          instructions: instructions || undefined,
          ...(startMode === 'interactive' && { interactive: true }),
          ...(startMode === 'plan' && { plan_mode: true }),
        },
      })

      if ((startMode === 'interactive' || startMode === 'plan') && result.session_id && onInteractiveStart) {
        onInteractiveStart(
          result.session_id,
          startMode === 'plan' ? 'planner' : l0AgentType,
        )
      }

      onClose()
      setInstructions('')
    } catch (error) {
      if (error instanceof ApiError && error.status === 409 && error.message.includes('concurrent ticket workflows')) {
        setShowConcurrentWarning(true)
        return
      }
      // Other errors handled by mutation state
    }
  }

  const handleForceRun = async () => {
    setShowConcurrentWarning(false)
    try {
      const result = await runMutation.mutateAsync({
        ticketId,
        params: {
          workflow: selectedWorkflow,
          instructions: instructions || undefined,
          ...(startMode === 'interactive' && { interactive: true }),
          ...(startMode === 'plan' && { plan_mode: true }),
          force: true,
        },
      })

      if ((startMode === 'interactive' || startMode === 'plan') && result.session_id && onInteractiveStart) {
        onInteractiveStart(
          result.session_id,
          startMode === 'plan' ? 'planner' : l0AgentType,
        )
      }

      onClose()
      setInstructions('')
    } catch {
      // Error handled by mutation state
    }
  }

  // Reset state when dialog closes
  useEffect(() => {
    if (!open) {
      setSelectedWorkflow('')
      setInstructions('')
      setStartMode('normal')
      setShowConcurrentWarning(false)
    }
  }, [open])

  return (
    <Dialog open={open} onClose={onClose} className="max-w-4xl max-h-[90vh]">
      <DialogHeader onClose={onClose}>
        <h2 className="text-lg font-semibold flex items-center gap-2">
          <Play className="h-5 w-5" />
          Run Workflow
        </h2>
      </DialogHeader>

      <DialogBody className="space-y-4">
        {isLoading ? (
          <div className="flex justify-center py-8">
            <Spinner />
          </div>
        ) : workflowIds.length === 0 ? (
          <p className="text-muted-foreground text-sm text-center py-4">
            No workflow definitions found. Create one on the Workflows page.
          </p>
        ) : (
          <>
            <div>
              <label className="block text-sm font-medium mb-1.5">Workflow</label>
              <Dropdown
                value={selectedWorkflow}
                onChange={setSelectedWorkflow}
                options={workflowIds.map((id) => ({
                  value: id,
                  label: id + (workflowDefs![id].description ? ` - ${workflowDefs![id].description}` : ''),
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
                  onChange={(e) => setInstructions(e.target.value)}
                  placeholder="Additional context or instructions for the agents..."
                  rows={6}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring resize-none"
                />
              </div>
            )}
          </>
        )}

        {blockedReason && (
          <div className="flex items-center gap-2 rounded-lg border border-yellow-200 bg-yellow-50 px-4 py-3 text-sm text-yellow-800 dark:border-yellow-800 dark:bg-yellow-950/30 dark:text-yellow-300">
            <span>{blockedReason}</span>
          </div>
        )}

        {showConcurrentWarning && (
          <div className="rounded-lg border border-yellow-200 bg-yellow-50 px-4 py-3 dark:border-yellow-800 dark:bg-yellow-950/30">
            <div className="flex items-center gap-2 text-sm font-medium text-yellow-800 dark:text-yellow-300">
              <AlertTriangle className="h-4 w-4 shrink-0" />
              Concurrent workflows without worktree isolation
            </div>
            <p className="mt-1 text-sm text-yellow-700 dark:text-yellow-400">
              Git worktrees are disabled. Running multiple ticket workflows concurrently without worktree isolation can cause file conflicts and git state corruption.
            </p>
            <div className="mt-3 flex gap-2">
              <Button variant="ghost" size="sm" onClick={onClose}>Cancel</Button>
              <Button variant="destructive" size="sm" onClick={handleForceRun}>
                {runMutation.isPending && <Spinner size="sm" className="mr-2" />}
                Proceed Anyway
              </Button>
            </div>
          </div>
        )}

        {runMutation.isError && !showConcurrentWarning && (
          <p className="text-sm text-destructive">
            {runMutation.error instanceof Error
              ? runMutation.error.message
              : 'Failed to start workflow'}
          </p>
        )}
      </DialogBody>

      <DialogFooter>
        <Button variant="ghost" onClick={onClose}>
          Cancel
        </Button>
        <Button
          onClick={handleRun}
          disabled={!selectedWorkflow || runMutation.isPending || !!blockedReason}
        >
          {runMutation.isPending && <Spinner size="sm" className="mr-2" />}
          Run
        </Button>
      </DialogFooter>
    </Dialog>
  )
}
