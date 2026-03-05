import { useState, useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Play } from 'lucide-react'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { Spinner } from '@/components/ui/Spinner'
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
}

export function RunWorkflowDialog({ open, onClose, ticketId, onInteractiveStart }: RunWorkflowDialogProps) {
  const [selectedWorkflow, setSelectedWorkflow] = useState('')
  const [instructions, setInstructions] = useState('')
  const [startMode, setStartMode] = useState<StartMode>('normal')

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
    } catch {
      // Error is handled by mutation state
    }
  }

  // Reset state when dialog closes
  useEffect(() => {
    if (!open) {
      setSelectedWorkflow('')
      setInstructions('')
      setStartMode('normal')
    }
  }, [open])

  return (
    <Dialog open={open} onClose={onClose} className="max-w-2xl">
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
                  <label className="flex items-center gap-2 text-sm cursor-pointer">
                    <input
                      type="checkbox"
                      checked={startMode === 'interactive'}
                      onChange={(e) => setStartMode(e.target.checked ? 'interactive' : 'normal')}
                      className="rounded border-input"
                    />
                    Start Interactive
                  </label>
                )}
                {canPlan && (
                  <label className="flex items-center gap-2 text-sm cursor-pointer">
                    <input
                      type="checkbox"
                      checked={startMode === 'plan'}
                      onChange={(e) => setStartMode(e.target.checked ? 'plan' : 'normal')}
                      className="rounded border-input"
                    />
                    Plan Before Execution
                  </label>
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
                  rows={4}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring resize-none"
                />
              </div>
            )}
          </>
        )}

        {runMutation.isError && (
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
          disabled={!selectedWorkflow || runMutation.isPending}
        >
          {runMutation.isPending && <Spinner size="sm" className="mr-2" />}
          Run
        </Button>
      </DialogFooter>
    </Dialog>
  )
}
