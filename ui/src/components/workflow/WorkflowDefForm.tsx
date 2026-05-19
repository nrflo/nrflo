import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { X } from 'lucide-react'
import { listWorkflowDefs } from '@/api/workflows'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Textarea } from '@/components/ui/Textarea'
import { Toggle } from '@/components/ui/Toggle'
import { Dropdown } from '@/components/ui/Dropdown'
import { useProjectStore } from '@/stores/projectStore'
import { useCLIModels } from '@/hooks/useCLIModels'
import type { ScopeType, WorkflowDefCreateRequest, WorkflowDefUpdateRequest } from '@/types/workflow'

const TAG_PATTERN = /^[a-zA-Z0-9-]+$/

interface WorkflowDefFormProps {
  initial?: {
    id: string
    description?: string
    scope_type?: ScopeType
    groups?: string[]
    close_ticket_on_complete?: boolean
    next_workflow_on_success?: string
    observer_context?: string
    observer_provider?: string | null
    observer_model?: string | null
  }
  isCreate: boolean
  onSubmit: (data: WorkflowDefCreateRequest | WorkflowDefUpdateRequest) => void
  formId?: string
}

export function WorkflowDefForm({ initial, isCreate, onSubmit, formId }: WorkflowDefFormProps) {
  const [id, setId] = useState(initial?.id || '')
  const [description, setDescription] = useState(initial?.description || '')
  const [scopeType, setScopeType] = useState<ScopeType>(initial?.scope_type || 'ticket')
  const [groups, setGroups] = useState<string[]>(initial?.groups || [])
  const [groupInput, setGroupInput] = useState('')
  const [closeTicketOnComplete, setCloseTicketOnComplete] = useState(initial?.close_ticket_on_complete ?? true)
  const [nextWorkflowOnSuccess, setNextWorkflowOnSuccess] = useState(initial?.next_workflow_on_success || '')
  const [observerContext, setObserverContext] = useState(initial?.observer_context || '')
  const [observerProvider, setObserverProvider] = useState(initial?.observer_provider || '')
  const [observerModel, setObserverModel] = useState(initial?.observer_model || '')

  const project = useProjectStore((s) => s.currentProject)
  const { data: models = [] } = useCLIModels()

  const { data: workflowDefs } = useQuery({
    queryKey: ['workflows', 'defs', project],
    queryFn: listWorkflowDefs,
  })

  const projectWorkflowOptions = useMemo(() => {
    if (!workflowDefs) return []
    return Object.entries(workflowDefs)
      .filter(([id, def]) => def.scope_type === 'project' && id !== initial?.id)
      .map(([id, def]) => ({ value: id, label: id + (def.description ? ` — ${def.description}` : '') }))
  }, [workflowDefs, initial?.id])

  const providerOptions = useMemo(() => {
    const types = Array.from(new Set(models.filter(m => m.enabled).map(m => m.cli_type)))
    return [{ value: '', label: 'Inherit project default' }, ...types.map(t => ({ value: t, label: t.charAt(0).toUpperCase() + t.slice(1) }))]
  }, [models])

  const modelOptions = useMemo(() => {
    const filtered = models.filter(m => m.enabled && (!observerProvider || m.cli_type === observerProvider))
    return [{ value: '', label: 'Inherit project default' }, ...filtered.map(m => ({ value: m.id, label: m.display_name }))]
  }, [models, observerProvider])

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
    const shared = {
      description: description.trim() || undefined,
      scope_type: scopeType,
      groups,
      close_ticket_on_complete: closeTicketOnComplete,
      next_workflow_on_success: nextWorkflowOnSuccess || undefined,
      observer_context: observerContext.trim() || undefined,
      observer_provider: observerProvider || null,
      observer_model: observerModel || null,
    }
    if (isCreate) {
      onSubmit({ id: id.trim(), ...shared } as WorkflowDefCreateRequest)
    } else {
      onSubmit(shared as WorkflowDefUpdateRequest)
    }
  }

  return (
    <form id={formId} onSubmit={handleSubmit} className="space-y-4">
      {isCreate && (
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">
            Workflow ID
          </label>
          <Input
            type="text"
            value={id}
            onChange={(e) => setId(e.target.value)}
            placeholder="e.g., feature, bugfix, hotfix"
            required
          />
        </div>
      )}

      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">
          Description
        </label>
        <Input
          type="text"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Short description of the workflow"
        />
      </div>

      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">
          Scope
        </label>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant={scopeType === 'ticket' ? 'default' : 'outline'}
            size="sm"
            onClick={() => setScopeType('ticket')}
          >
            Ticket
          </Button>
          <Button
            type="button"
            variant={scopeType === 'project' ? 'default' : 'outline'}
            size="sm"
            onClick={() => setScopeType('project')}
          >
            Project
          </Button>
        </div>
        {scopeType === 'project' && (
          <p className="text-xs text-muted-foreground mt-1">
            Project workflows run without a ticket. Ticket template variables are unavailable.
          </p>
        )}
      </div>

      {scopeType === 'ticket' && (
        <Toggle
          checked={closeTicketOnComplete}
          onChange={setCloseTicketOnComplete}
          label="Close ticket after workflow finished"
        />
      )}

      <div>
        <Toggle
          checked={nextWorkflowOnSuccess !== ''}
          onChange={(checked) => {
            if (checked) {
              setNextWorkflowOnSuccess(projectWorkflowOptions[0]?.value ?? '')
            } else {
              setNextWorkflowOnSuccess('')
            }
          }}
          label="Run another workflow on success"
          disabled={projectWorkflowOptions.length === 0}
        />
        {projectWorkflowOptions.length === 0 ? (
          <p className="text-xs text-muted-foreground mt-1">Create a project-scoped workflow first.</p>
        ) : nextWorkflowOnSuccess !== '' ? (
          <div className="mt-2">
            <Dropdown
              value={nextWorkflowOnSuccess}
              onChange={setNextWorkflowOnSuccess}
              options={projectWorkflowOptions}
            />
          </div>
        ) : null}
        <p className="text-xs text-muted-foreground mt-1">
          Pipes <code>workflow_final_result</code> into the next workflow&apos;s instructions. Skipped when no result is produced.
        </p>
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
        <Input
          type="text"
          value={groupInput}
          onChange={(e) => setGroupInput(e.target.value)}
          onKeyDown={handleGroupKeyDown}
          placeholder="Type a tag and press Enter"
        />
        <p className="text-xs text-muted-foreground mt-1">
          Define tag groups for skip logic (e.g., be, fe, docs). Press Enter or comma to add.
        </p>
      </div>

      <div className="border-t border-border pt-3 space-y-3">
        <div className="text-xs font-medium text-muted-foreground">Observer overrides</div>
        <div>
          <label className="block text-xs font-medium text-muted-foreground mb-1">
            Observer context
          </label>
          <Textarea
            value={observerContext}
            onChange={(e) => setObserverContext(e.target.value)}
            rows={2}
            placeholder="Optional observer context for this workflow (overrides project setting)"
          />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="block text-xs font-medium text-muted-foreground mb-1">Provider</label>
            <Dropdown
              value={observerProvider}
              onChange={(v) => { setObserverProvider(v); setObserverModel('') }}
              options={providerOptions}
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-muted-foreground mb-1">Model</label>
            <Dropdown
              value={observerModel}
              onChange={setObserverModel}
              options={modelOptions}
            />
          </div>
        </div>
      </div>
    </form>
  )
}
