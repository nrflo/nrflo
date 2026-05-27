import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { X } from 'lucide-react'
import { listWorkflowDefs } from '@/api/workflows'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Toggle } from '@/components/ui/Toggle'
import { Dropdown } from '@/components/ui/Dropdown'
import { FinalizeSection, type FinalizeState } from '@/components/workflow/WorkflowDefForm.finalize-slot'
import { PauseSection, type PauseState } from '@/components/workflow/WorkflowDefForm.pause-slot'
import { ObserverSection, type ObserverState } from '@/components/workflow/WorkflowDefForm.observer-slot'
import {
  FindingSchemasSection,
  findingSchemasToRows,
  parseFindingSchemaRows,
  type FindingSchemaRow,
} from '@/components/workflow/WorkflowDefForm.finding-schemas-slot'
import { useProjectStore } from '@/stores/projectStore'
import type { FindingSchema, ScopeType, WorkflowDefCreateRequest, WorkflowDefUpdateRequest } from '@/types/workflow'

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
    finalize_success_command?: string
    finalize_success_script_id?: string
    finalize_failure_command?: string
    finalize_failure_script_id?: string
    pause_event_command?: string
    pause_event_script_id?: string
    finding_schemas?: FindingSchema[]
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
  const [observer, setObserver] = useState<ObserverState>({
    context: initial?.observer_context || '',
    provider: initial?.observer_provider || '',
    model: initial?.observer_model || '',
  })
  const [findingSchemas, setFindingSchemas] = useState<FindingSchemaRow[]>(findingSchemasToRows(initial?.finding_schemas))
  const [schemaError, setSchemaError] = useState<string | null>(null)
  const [finalize, setFinalize] = useState<FinalizeState>({
    successMode: initial?.finalize_success_command ? 'command' : initial?.finalize_success_script_id ? 'script' : '',
    successCommand: initial?.finalize_success_command || '',
    successScriptId: initial?.finalize_success_script_id || '',
    failureMode: initial?.finalize_failure_command ? 'command' : initial?.finalize_failure_script_id ? 'script' : '',
    failureCommand: initial?.finalize_failure_command || '',
    failureScriptId: initial?.finalize_failure_script_id || '',
  })
  const [pause, setPause] = useState<PauseState>({
    mode: initial?.pause_event_command ? 'command' : initial?.pause_event_script_id ? 'script' : '',
    command: initial?.pause_event_command || '',
    scriptId: initial?.pause_event_script_id || '',
  })

  const project = useProjectStore((s) => s.currentProject)

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
    const { schemas, error } = parseFindingSchemaRows(findingSchemas)
    if (error) {
      setSchemaError(error)
      return
    }
    setSchemaError(null)
    const shared = {
      description: description.trim() || undefined,
      scope_type: scopeType,
      groups,
      close_ticket_on_complete: closeTicketOnComplete,
      next_workflow_on_success: nextWorkflowOnSuccess || undefined,
      observer_context: observer.context.trim() || undefined,
      observer_provider: observer.provider || null,
      observer_model: observer.model || null,
      finding_schemas: schemas,
      finalize_success_command: finalize.successMode === 'command' ? finalize.successCommand || undefined : undefined,
      finalize_success_script_id: finalize.successMode === 'script' ? finalize.successScriptId || undefined : undefined,
      finalize_failure_command: finalize.failureMode === 'command' ? finalize.failureCommand || undefined : undefined,
      finalize_failure_script_id: finalize.failureMode === 'script' ? finalize.failureScriptId || undefined : undefined,
      pause_event_command: pause.mode === 'command' ? pause.command || undefined : undefined,
      pause_event_script_id: pause.mode === 'script' ? pause.scriptId || undefined : undefined,
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

      <ObserverSection state={observer} onChange={(patch) => setObserver((prev) => ({ ...prev, ...patch }))} />

      <FinalizeSection state={finalize} onChange={(patch) => setFinalize((prev) => ({ ...prev, ...patch }))} />
      <PauseSection state={pause} onChange={(patch) => setPause((prev) => ({ ...prev, ...patch }))} />
      <FindingSchemasSection rows={findingSchemas} onChange={setFindingSchemas} />
      {schemaError && <p className="text-xs text-destructive">{schemaError}</p>}
    </form>
  )
}

