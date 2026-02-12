import { useState } from 'react'
import { X } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { PhaseListEditor, type PhaseFormEntry } from '@/components/workflow/PhaseListEditor'
import type { PhaseDef, WorkflowDefCreateRequest, WorkflowDefUpdateRequest } from '@/types/workflow'

const PRESET_CATEGORIES = ['full', 'simple', 'docs']

/** Convert API phase format to form entries */
function phasesToForm(phases?: PhaseDef[]): PhaseFormEntry[] {
  if (!phases?.length) return [{ agent: '', layer: 0, skip_for: [] }]
  return phases.map((p) => ({
    agent: p.agent || p.id,
    layer: p.layer ?? 0,
    skip_for: p.skip_for || [],
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
      ...(e.skip_for.length > 0 ? { skip_for: e.skip_for } : {}),
    }))
}

interface WorkflowDefFormProps {
  initial?: { id: string; description?: string; categories?: string[]; phases?: PhaseDef[] }
  isCreate: boolean
  onSubmit: (data: WorkflowDefCreateRequest | WorkflowDefUpdateRequest) => void
  onCancel: () => void
  isPending?: boolean
}

export function WorkflowDefForm({ initial, isCreate, onSubmit, onCancel, isPending }: WorkflowDefFormProps) {
  const [id, setId] = useState(initial?.id || '')
  const [description, setDescription] = useState(initial?.description || '')
  const [categories, setCategories] = useState<string[]>(initial?.categories || ['full'])
  const [catInput, setCatInput] = useState('')
  const [phases, setPhases] = useState<PhaseFormEntry[]>(phasesToForm(initial?.phases))

  const addCategory = (cat: string) => {
    const val = cat.trim()
    if (!val || categories.includes(val)) return
    setCategories([...categories, val])
  }

  const removeCategory = (cat: string) => {
    setCategories(categories.filter((c) => c !== cat))
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const apiPhases = formToPhases(phases)
    if (isCreate) {
      onSubmit({
        id: id.trim(),
        description: description.trim() || undefined,
        categories: categories.length ? categories : undefined,
        phases: apiPhases,
      } as WorkflowDefCreateRequest)
    } else {
      onSubmit({
        description: description.trim() || undefined,
        categories: categories.length ? categories : undefined,
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
          Categories
        </label>
        <div className="flex flex-wrap items-center gap-1.5 mb-1.5">
          {categories.map((cat) => (
            <Badge key={cat} variant="secondary" className="text-xs gap-1 pr-1">
              {cat}
              <button type="button" onClick={() => removeCategory(cat)} className="hover:text-destructive">
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ))}
        </div>
        <div className="flex items-center gap-1.5">
          {PRESET_CATEGORIES.filter((c) => !categories.includes(c)).map((cat) => (
            <button
              key={cat}
              type="button"
              onClick={() => addCategory(cat)}
              className="text-xs px-2 py-0.5 rounded border border-dashed border-border text-muted-foreground hover:text-foreground hover:border-foreground transition-colors"
            >
              +{cat}
            </button>
          ))}
          <input
            type="text"
            value={catInput}
            onChange={(e) => setCatInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                if (catInput.trim()) {
                  addCategory(catInput)
                  setCatInput('')
                }
              }
            }}
            placeholder="Custom..."
            className="w-24 rounded border border-border bg-background px-2 py-0.5 text-xs"
          />
        </div>
      </div>

      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">
          Agents
        </label>
        <PhaseListEditor value={phases} onChange={setPhases} categories={categories} />
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
