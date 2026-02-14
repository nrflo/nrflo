import { useState } from 'react'
import { Button } from '@/components/ui/Button'
import { PhaseListEditor, type PhaseFormEntry } from '@/components/workflow/PhaseListEditor'
import type { PhaseDef, ScopeType, WorkflowDefCreateRequest, WorkflowDefUpdateRequest } from '@/types/workflow'

/** Convert API phase format to form entries */
function phasesToForm(phases?: PhaseDef[]): PhaseFormEntry[] {
  if (!phases?.length) return [{ agent: '', layer: 0 }]
  return phases.map((p) => ({
    agent: p.agent || p.id,
    layer: p.layer ?? 0,
  }))
}

/** Convert form entries to API format (always object with layer) */
function formToPhases(entries: PhaseFormEntry[]): PhaseDef[] {
  return entries
    .filter((e) => e.agent.trim())
    .map((e) => ({
      id: e.agent.trim(),
      agent: e.agent.trim(),
      layer: e.layer,
    }))
}

interface WorkflowDefFormProps {
  initial?: { id: string; description?: string; scope_type?: ScopeType; phases?: PhaseDef[] }
  isCreate: boolean
  onSubmit: (data: WorkflowDefCreateRequest | WorkflowDefUpdateRequest) => void
  onCancel: () => void
  isPending?: boolean
}

export function WorkflowDefForm({ initial, isCreate, onSubmit, onCancel, isPending }: WorkflowDefFormProps) {
  const [id, setId] = useState(initial?.id || '')
  const [description, setDescription] = useState(initial?.description || '')
  const [scopeType, setScopeType] = useState<ScopeType>(initial?.scope_type || 'ticket')
  const [phases, setPhases] = useState<PhaseFormEntry[]>(phasesToForm(initial?.phases))

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const apiPhases = formToPhases(phases)
    if (isCreate) {
      onSubmit({
        id: id.trim(),
        description: description.trim() || undefined,
        scope_type: scopeType,
        phases: apiPhases,
      } as WorkflowDefCreateRequest)
    } else {
      onSubmit({
        description: description.trim() || undefined,
        scope_type: scopeType,
        phases: apiPhases.length ? apiPhases : undefined,
      } as WorkflowDefUpdateRequest)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {isCreate && (
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">
            Workflow ID
          </label>
          <input
            type="text"
            value={id}
            onChange={(e) => setId(e.target.value)}
            placeholder="e.g., feature, bugfix, hotfix"
            required
            className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
          />
        </div>
      )}

      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">
          Description
        </label>
        <input
          type="text"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Short description of the workflow"
          className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
        />
      </div>

      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">
          Scope
        </label>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setScopeType('ticket')}
            className={`text-xs px-3 py-1 rounded border transition-colors ${
              scopeType === 'ticket'
                ? 'border-primary bg-primary/10 text-primary'
                : 'border-border text-muted-foreground hover:text-foreground'
            }`}
          >
            Ticket
          </button>
          <button
            type="button"
            onClick={() => setScopeType('project')}
            className={`text-xs px-3 py-1 rounded border transition-colors ${
              scopeType === 'project'
                ? 'border-primary bg-primary/10 text-primary'
                : 'border-border text-muted-foreground hover:text-foreground'
            }`}
          >
            Project
          </button>
        </div>
        {scopeType === 'project' && (
          <p className="text-xs text-muted-foreground mt-1">
            Project workflows run without a ticket. Ticket template variables are unavailable.
          </p>
        )}
      </div>

      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">
          Agents
        </label>
        <PhaseListEditor value={phases} onChange={setPhases} />
      </div>

      <div className="flex gap-2 justify-end pt-2">
        <Button type="button" variant="ghost" size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" size="sm" disabled={isPending}>
          {isPending ? 'Saving...' : isCreate ? 'Create Workflow' : 'Save Changes'}
        </Button>
      </div>
    </form>
  )
}
