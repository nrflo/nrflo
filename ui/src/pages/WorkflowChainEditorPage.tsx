import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, Plus } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Spinner } from '@/components/ui/Spinner'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { useIsAdmin } from '@/stores/authStore'
import { useProjectStore } from '@/stores/projectStore'
import { listWorkflowDefs } from '@/api/workflows'
import {
  useWorkflowChain,
  useUpdateWorkflowChain,
  useAppendStep,
  useUpdateStep,
  useDeleteStep,
  useReorderSteps,
} from '@/hooks/useWorkflowChains'
import { StepRow } from './WorkflowChainStepRow'
import type { StepEdit } from './WorkflowChainStepRow'
import type { WorkflowChainStep } from '@/types/workflowChain'
import { WorkflowChainRunsTab } from './WorkflowChainRunsTab'

export function WorkflowChainEditorPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const isAdmin = useIsAdmin()
  const currentProject = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  const [activeTab, setActiveTab] = useState<'definition' | 'runs'>('definition')

  const { data: chainData, isLoading } = useWorkflowChain(id ?? '')

  const { data: workflowDefs } = useQuery({
    queryKey: ['workflows', 'defs', currentProject],
    queryFn: listWorkflowDefs,
    enabled: projectsLoaded,
  })

  const workflowOptions = workflowDefs
    ? Object.keys(workflowDefs).map((name) => ({ value: name, label: name }))
    : []

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [stepEdits, setStepEdits] = useState<Record<string, StepEdit>>({})
  const [savingStepId, setSavingStepId] = useState<string | null>(null)
  const [deleteStepTarget, setDeleteStepTarget] = useState<WorkflowChainStep | null>(null)

  useEffect(() => {
    if (chainData) {
      setName(chainData.name)
      setDescription(chainData.description)
      const edits: Record<string, StepEdit> = {}
      for (const step of chainData.steps) {
        edits[step.id] = {
          workflow_name: step.workflow_name,
          scope_type: step.scope_type,
          base_instructions: step.base_instructions,
          require_ticket_handoff: step.require_ticket_handoff,
        }
      }
      setStepEdits(edits)
    }
  }, [chainData])

  const updateChainMutation = useUpdateWorkflowChain()
  const appendStepMutation = useAppendStep()
  const updateStepMutation = useUpdateStep()
  const deleteStepMutation = useDeleteStep()
  const reorderStepsMutation = useReorderSteps()

  if (!id) return null

  const steps = chainData?.steps ?? []

  const handleEditStep = (stepId: string, field: keyof StepEdit, value: string | boolean) => {
    setStepEdits((prev) => ({
      ...prev,
      [stepId]: { ...prev[stepId], [field]: value },
    }))
  }

  const handleSaveChain = () => {
    updateChainMutation.mutate(
      { id, data: { name: name.trim(), description: description.trim() } },
      {
        onSuccess: () => toast.success('Chain saved'),
        onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to save chain'),
      }
    )
  }

  const handleSaveStep = (stepId: string) => {
    const edit = stepEdits[stepId]
    if (!edit) return
    setSavingStepId(stepId)
    updateStepMutation.mutate(
      { chainId: id, stepId, data: edit },
      {
        onSuccess: () => toast.success('Step saved'),
        onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to save step'),
        onSettled: () => setSavingStepId(null),
      }
    )
  }

  const handleMoveUp = (index: number) => {
    if (index === 0) return
    const newOrder = [...steps]
    ;[newOrder[index - 1], newOrder[index]] = [newOrder[index], newOrder[index - 1]]
    reorderStepsMutation.mutate(
      { chainId: id, data: { ordered_step_ids: newOrder.map((s) => s.id) } },
      {
        onError: (err) =>
          toast.error(err instanceof Error ? err.message : 'Failed to reorder steps'),
      }
    )
  }

  const handleMoveDown = (index: number) => {
    if (index === steps.length - 1) return
    const newOrder = [...steps]
    ;[newOrder[index], newOrder[index + 1]] = [newOrder[index + 1], newOrder[index]]
    reorderStepsMutation.mutate(
      { chainId: id, data: { ordered_step_ids: newOrder.map((s) => s.id) } },
      {
        onError: (err) =>
          toast.error(err instanceof Error ? err.message : 'Failed to reorder steps'),
      }
    )
  }

  const handleAddStep = () => {
    const isFirstStep = steps.length === 0
    appendStepMutation.mutate(
      {
        chainId: id,
        data: {
          workflow_name: workflowOptions[0]?.value ?? '',
          scope_type: isFirstStep ? 'project' : 'ticket',
          base_instructions: '',
          require_ticket_handoff: false,
        },
      },
      {
        onError: (err) =>
          toast.error(err instanceof Error ? err.message : 'Failed to add step'),
      }
    )
  }

  const confirmDeleteStep = () => {
    if (!deleteStepTarget) return
    const targetId = deleteStepTarget.id
    deleteStepMutation.mutate(
      { chainId: id, stepId: targetId },
      {
        onSuccess: () => {
          setDeleteStepTarget(null)
          setStepEdits((prev) => {
            const next = { ...prev }
            delete next[targetId]
            return next
          })
        },
        onError: (err) => {
          setDeleteStepTarget(null)
          toast.error(err instanceof Error ? err.message : 'Failed to delete step')
        },
      }
    )
  }

  return (
    <div className="max-w-3xl mx-auto space-y-6">
      <div className="flex items-center gap-3">
        <button
          onClick={() => navigate('/workflow-chains')}
          className="p-1 text-muted-foreground hover:text-foreground transition-colors"
          title="Back to chains"
        >
          <ArrowLeft className="h-4 w-4" />
        </button>
        <h1 className="text-2xl font-bold tracking-tight">
          {isLoading ? 'Loading…' : (chainData?.name ?? 'Workflow Chain')}
        </h1>
      </div>

      <div className="flex gap-4 border-b border-border">
        {(['definition', 'runs'] as const).map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={`pb-2 text-sm font-medium capitalize transition-colors border-b-2 -mb-px ${
              activeTab === tab
                ? 'border-primary text-foreground'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            }`}
          >
            {tab}
          </button>
        ))}
      </div>

      {activeTab === 'runs' && id ? (
        <WorkflowChainRunsTab chainId={id} />
      ) : isLoading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : !chainData ? (
        <p className="text-destructive text-sm">Workflow chain not found.</p>
      ) : (
        <>
          <div className="border border-border rounded-lg p-4 space-y-4">
            <h2 className="text-base font-semibold">Chain Details</h2>
            <div className="space-y-1.5">
              <label className="text-sm font-medium">Name</label>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Chain name"
                disabled={!isAdmin}
              />
            </div>
            <div className="space-y-1.5">
              <label className="text-sm font-medium">Description</label>
              <Input
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="Optional description"
                disabled={!isAdmin}
              />
            </div>
            {isAdmin && (
              <div className="flex justify-end">
                <Button
                  onClick={handleSaveChain}
                  disabled={!name.trim() || updateChainMutation.isPending}
                >
                  {updateChainMutation.isPending ? 'Saving…' : 'Save chain'}
                </Button>
              </div>
            )}
          </div>

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <h2 className="text-base font-semibold">
                Steps{steps.length > 0 ? ` (${steps.length})` : ''}
              </h2>
              {isAdmin && (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={handleAddStep}
                  disabled={appendStepMutation.isPending}
                >
                  <Plus className="h-4 w-4 mr-1" />
                  {appendStepMutation.isPending ? 'Adding…' : 'Add step'}
                </Button>
              )}
            </div>

            {steps.length === 0 ? (
              <div className="text-center py-8 text-muted-foreground border border-dashed border-border rounded-lg">
                <p className="text-sm">No steps yet. Add one to get started.</p>
              </div>
            ) : (
              <div className="space-y-3">
                {steps.map((step, index) => {
                  const edit = stepEdits[step.id] ?? {
                    workflow_name: step.workflow_name,
                    scope_type: step.scope_type,
                    base_instructions: step.base_instructions,
                    require_ticket_handoff: step.require_ticket_handoff,
                  }
                  return (
                    <StepRow
                      key={step.id}
                      step={step}
                      edit={edit}
                      index={index}
                      total={steps.length}
                      workflowOptions={workflowOptions}
                      isAdmin={isAdmin}
                      isPendingReorder={reorderStepsMutation.isPending}
                      onEdit={handleEditStep}
                      onSave={handleSaveStep}
                      onMoveUp={handleMoveUp}
                      onMoveDown={handleMoveDown}
                      onDelete={setDeleteStepTarget}
                      isSavingStep={savingStepId === step.id}
                    />
                  )
                })}
              </div>
            )}
          </div>
        </>
      )}

      <ConfirmDialog
        open={!!deleteStepTarget}
        onClose={() => setDeleteStepTarget(null)}
        onConfirm={confirmDeleteStep}
        title="Delete Step"
        message="Are you sure you want to delete this step? This action cannot be undone."
        confirmLabel="Delete"
        variant="destructive"
      />
    </div>
  )
}
