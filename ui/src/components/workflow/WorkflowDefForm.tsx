import { useState } from 'react'
import { X } from 'lucide-react'
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

const TAG_PATTERN = /^[a-zA-Z0-9-]+$/

interface WorkflowDefFormProps {
  initial?: { id: string; description?: string; scope_type?: ScopeType; groups?: string[]; phases?: PhaseDef[] }
  isCreate: boolean
  onSubmit: (data: WorkflowDefCreateRequest | WorkflowDefUpdateRequest) => void
  onCancel: () => void
  isPending?: boolean
}

export function WorkflowDefForm({ initial, isCreate, onSubmit, onCancel, isPending }: WorkflowDefFormProps) {
  const [id, setId] = useState(initial?.id || '')
  const [description, setDescription] = useState(initial?.description || '')
  const [scopeType, setScopeType] = useState<ScopeType>(initial?.scope_type || 'ticket')
  const [groups, setGroups] = useState<string[]>(initial?.groups || [])
  const [groupInput, setGroupInput] = useState('')
  const [phases, setPhases] = useState<PhaseFormEntry[]>(phasesToForm(initial?.phases))

  const addGroup = (raw: string) => {
    const tag = raw.trim().toLowerCase()
    if (!tag || !TAG_PATTERN.test(tag) || groups.includes(tag)) return
    setGroups([...groups, tag])
    setGroupInput('')
  }

  const handleGroupKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault()
      addGroup(groupInput)
    }
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const apiPhases = formToPhases(phases)
    if (isCreate) {
      onSubmit({
        id: id.trim(),
        description: description.trim() || undefined,
        scope_type: scopeType,
        groups,
        phases: apiPhases,
      } as WorkflowDefCreateRequest)
    } else {
      onSubmit({
        description: description.trim() || undefined,
        scope_type: scopeType,
        groups,
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
          Groups (Tags)
        </label>
        <div className="flex flex-wrap gap-1.5 mb-1.5">
          {groups.map((g) => (
            <span
              key={g}
              className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full border border-border bg-muted"
            >
              {g}
              <button
                type="button"
                onClick={() => setGroups(groups.filter((t) => t !== g))}
                className="hover:text-destructive"
                aria-label={`Remove ${g}`}
              >
                <X className="h-3 w-3" />
              </button>
            </span>
          ))}
        </div>
        <input
          type="text"
          value={groupInput}
          onChange={(e) => setGroupInput(e.target.value)}
          onKeyDown={handleGroupKeyDown}
          placeholder="Type a tag and press Enter"
          className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
        />
        <p className="text-xs text-muted-foreground mt-1">
          Define tag groups for skip logic (e.g., be, fe, docs). Press Enter or comma to add.
        </p>
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
