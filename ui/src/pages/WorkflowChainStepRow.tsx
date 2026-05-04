import { ChevronUp, ChevronDown, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Textarea } from '@/components/ui/Textarea'
import { Dropdown } from '@/components/ui/Dropdown'
import { Toggle } from '@/components/ui/Toggle'
import type { WorkflowChainStep } from '@/types/workflowChain'

export interface StepEdit {
  workflow_name: string
  scope_type: 'project' | 'ticket'
  base_instructions: string
  require_ticket_handoff: boolean
}

const SCOPE_OPTIONS = [
  { value: 'project', label: 'Project' },
  { value: 'ticket', label: 'Ticket' },
]

interface StepRowProps {
  step: WorkflowChainStep
  edit: StepEdit
  index: number
  total: number
  workflowOptions: { value: string; label: string }[]
  isAdmin: boolean
  isPendingReorder: boolean
  onEdit: (stepId: string, field: keyof StepEdit, value: string | boolean) => void
  onSave: (stepId: string) => void
  onMoveUp: (index: number) => void
  onMoveDown: (index: number) => void
  onDelete: (step: WorkflowChainStep) => void
  isSavingStep: boolean
}

export function StepRow({
  step,
  edit,
  index,
  total,
  workflowOptions,
  isAdmin,
  isPendingReorder,
  onEdit,
  onSave,
  onMoveUp,
  onMoveDown,
  onDelete,
  isSavingStep,
}: StepRowProps) {
  const isFirst = index === 0
  const scopeError = isFirst && edit.scope_type !== 'project'
    ? 'Step 0 must be project-scoped'
    : null
  const handoffError = edit.require_ticket_handoff && edit.scope_type !== 'ticket'
    ? 'Ticket handoff requires ticket scope'
    : null

  const canSaveStep = !scopeError && !handoffError

  return (
    <div className="border border-border rounded-lg p-4 space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-muted-foreground">Step {index}</span>
        <div className="flex items-center gap-1">
          {isAdmin && (
            <>
              <button
                onClick={() => onMoveUp(index)}
                disabled={index === 0 || isPendingReorder}
                className="p-1 text-muted-foreground hover:text-foreground disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                title="Move up"
              >
                <ChevronUp className="h-4 w-4" />
              </button>
              <button
                onClick={() => onMoveDown(index)}
                disabled={index === total - 1 || isPendingReorder}
                className="p-1 text-muted-foreground hover:text-foreground disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                title="Move down"
              >
                <ChevronDown className="h-4 w-4" />
              </button>
              <button
                onClick={() => onDelete(step)}
                className="p-1 text-muted-foreground hover:text-destructive transition-colors"
                title="Delete step"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            </>
          )}
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <label className="text-sm font-medium">Workflow</label>
          <Dropdown
            value={edit.workflow_name}
            onChange={(v) => onEdit(step.id, 'workflow_name', v)}
            options={workflowOptions}
            placeholder="Select workflow"
            disabled={!isAdmin}
          />
        </div>
        <div className="space-y-1.5">
          <label className="text-sm font-medium">Scope</label>
          <Dropdown
            value={edit.scope_type}
            onChange={(v) => onEdit(step.id, 'scope_type', v as 'project' | 'ticket')}
            options={SCOPE_OPTIONS}
            disabled={!isAdmin}
          />
          {scopeError && <p className="text-xs text-destructive">{scopeError}</p>}
        </div>
      </div>

      <div className="space-y-1.5">
        <label className="text-sm font-medium">Base Instructions</label>
        <Textarea
          value={edit.base_instructions}
          onChange={(e) => onEdit(step.id, 'base_instructions', e.target.value)}
          placeholder="Optional instructions for this step"
          rows={3}
          disabled={!isAdmin}
        />
      </div>

      <div className="flex items-center justify-between">
        <Toggle
          checked={edit.require_ticket_handoff}
          onChange={(v) => onEdit(step.id, 'require_ticket_handoff', v)}
          label="Require ticket handoff"
          disabled={!isAdmin || edit.scope_type !== 'ticket'}
        />
        {handoffError && <p className="text-xs text-destructive">{handoffError}</p>}
        {isAdmin && (
          <Button
            size="sm"
            onClick={() => onSave(step.id)}
            disabled={!canSaveStep || isSavingStep}
          >
            {isSavingStep ? 'Saving…' : 'Save step'}
          </Button>
        )}
      </div>
    </div>
  )
}
