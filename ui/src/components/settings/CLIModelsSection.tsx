import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Pencil, Trash2, Cpu, Lock } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/Card'
import { Badge } from '@/components/ui/Badge'
import {
  createCLIModel,
  updateCLIModel,
  deleteCLIModel,
  type CLIModel,
  type CreateCLIModelRequest,
  type UpdateCLIModelRequest,
} from '@/api/cliModels'
import { useCLIModels, cliModelKeys } from '@/hooks/useCLIModels'
import { Toggle } from '@/components/ui/Toggle'
import { CLIModelForm, emptyCLIModelForm, type CLIModelFormData } from './CLIModelForm'
import { CLIModelCheckButton } from './CLIModelCheckButton'

const MODEL_GROUPS = [
  { key: 'claude', label: 'Claude' },
  { key: 'codex', label: 'Codex' },
  { key: 'opencode', label: 'OpenCode' },
] as const

function groupModels(models: CLIModel[]) {
  const knownKeys = new Set<string>(MODEL_GROUPS.map((g) => g.key))
  const groups: { label: string; models: CLIModel[] }[] = []
  for (const g of MODEL_GROUPS) {
    const items = models.filter((m) => m.cli_type === g.key)
    if (items.length > 0) groups.push({ label: g.label, models: items })
  }
  const other = models.filter((m) => !knownKeys.has(m.cli_type))
  if (other.length > 0) groups.push({ label: 'Other', models: other })
  return groups
}

function cliTypeBadgeColor(cliType: string): string {
  switch (cliType) {
    case 'claude': return 'bg-blue-500/15 text-blue-600 dark:text-blue-400'
    case 'opencode': return 'bg-purple-500/15 text-purple-600 dark:text-purple-400'
    case 'codex': return 'bg-green-500/15 text-green-600 dark:text-green-400'
    default: return 'bg-muted text-muted-foreground'
  }
}

function modelToFormData(m: CLIModel): CLIModelFormData {
  return {
    id: m.id,
    cli_type: m.cli_type,
    display_name: m.display_name,
    mapped_model: m.mapped_model,
    reasoning_effort: m.reasoning_effort || '',
    context_length: String(m.context_length),
  }
}

export function CLIModelsSection() {
  const queryClient = useQueryClient()

  const [isCreating, setIsCreating] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [formData, setFormData] = useState<CLIModelFormData>(emptyCLIModelForm)
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null)
  const [toggleErrors, setToggleErrors] = useState<Record<string, string>>({})

  const { data: models = [], isLoading, error } = useCLIModels()

  const createMutation = useMutation({
    mutationFn: (data: CreateCLIModelRequest) => createCLIModel(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: cliModelKeys.list() })
      setIsCreating(false)
      setFormData(emptyCLIModelForm)
    },
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateCLIModelRequest }) =>
      updateCLIModel(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: cliModelKeys.list() })
      setEditingId(null)
      setFormData(emptyCLIModelForm)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteCLIModel(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: cliModelKeys.list() })
      setDeleteConfirm(null)
    },
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      updateCLIModel(id, { enabled }),
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: cliModelKeys.list() })
      setToggleErrors((prev) => { const next = { ...prev }; delete next[vars.id]; return next })
    },
    onError: (err, vars) => {
      setToggleErrors((prev) => ({ ...prev, [vars.id]: (err as Error).message }))
    },
  })

  const handleStartCreate = () => {
    setIsCreating(true)
    setEditingId(null)
    setFormData(emptyCLIModelForm)
  }

  const handleStartEdit = (m: CLIModel) => {
    setEditingId(m.id)
    setIsCreating(false)
    setFormData(modelToFormData(m))
  }

  const handleCancel = () => {
    setIsCreating(false)
    setEditingId(null)
    setFormData(emptyCLIModelForm)
  }

  const handleSaveCreate = () => {
    if (!formData.id.trim() || !formData.display_name.trim() || !formData.mapped_model.trim()) return
    const contextLength = parseInt(formData.context_length, 10)
    createMutation.mutate({
      id: formData.id.trim(),
      cli_type: formData.cli_type,
      display_name: formData.display_name.trim(),
      mapped_model: formData.mapped_model.trim(),
      reasoning_effort: formData.reasoning_effort.trim() || undefined,
      context_length: isNaN(contextLength) ? undefined : contextLength,
    })
  }

  const handleSaveEdit = () => {
    if (!editingId) return
    const contextLength = parseInt(formData.context_length, 10)
    updateMutation.mutate({
      id: editingId,
      data: {
        display_name: formData.display_name.trim(),
        mapped_model: formData.mapped_model.trim(),
        reasoning_effort: formData.reasoning_effort.trim() || undefined,
        context_length: isNaN(contextLength) ? undefined : contextLength,
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>CLI Models</CardTitle>
            <CardDescription>Manage CLI model configurations for agent definitions</CardDescription>
          </div>
          <Button onClick={handleStartCreate} disabled={isCreating}>
            <Plus className="h-4 w-4 mr-2" />
            New Model
          </Button>
        </div>
        <div className="text-sm text-muted-foreground mt-3 space-y-2">
          <p>
            <span className="font-medium">Model Check (⚡):</span> Spawns a minimal agent with a test prompt to verify the CLI binary and model config work. Fixed 60-second timeout. Returns success with duration or an error message.
          </p>
          <p>
            <span className="font-medium">Model Resolution:</span> Each model defines <code className="text-xs">cli_type</code>, <code className="text-xs">mapped_model</code>, <code className="text-xs">reasoning_effort</code>, and <code className="text-xs">context_length</code>. At workflow start, the spawner loads all cli_models into a config map and uses <code className="text-xs">mapped_model</code> + <code className="text-xs">reasoning_effort</code> from DB, falling back to adapter defaults. The <code className="text-xs">model</code> field on agent definitions is not validated against this table — only <code className="text-xs">low_consumption_model</code> is.
          </p>
          <p>
            <span className="font-medium">Timeouts:</span> Agent wall-clock timeout defaults to 40 min (per agent definition). Stall detection: start 120s, running 480s — configurable per agent and globally in General settings (per-agent &gt; global &gt; hardcoded default; 0 = disabled). Model check timeout is fixed at 60s.
          </p>
        </div>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">
          {isLoading && (
            <div className="text-center py-8 text-muted-foreground">Loading models...</div>
          )}
          {error && (
            <div className="text-center py-8 text-destructive">
              Error: {(error as Error).message}
            </div>
          )}

          {isCreating && (
            <CLIModelForm
              formData={formData}
              setFormData={setFormData}
              onCancel={handleCancel}
              onSave={handleSaveCreate}
              mutation={createMutation}
              isCreate
            />
          )}

          {!isLoading && !error && models.length === 0 && !isCreating && (
            <div className="text-center py-8 text-muted-foreground">
              No CLI models found. Create one to get started.
            </div>
          )}

          {groupModels(models).map((group) => (
            <div key={group.label}>
              <h3 className="text-sm font-semibold text-muted-foreground mt-4 mb-2">{group.label}</h3>
              {group.models.map((m) => (
                <div key={m.id} className="border rounded-lg p-4">
                  {editingId === m.id ? (
                    <CLIModelForm
                      formData={formData}
                      setFormData={setFormData}
                      onCancel={handleCancel}
                      onSave={handleSaveEdit}
                      mutation={updateMutation}
                    />
                  ) : deleteConfirm === m.id ? (
                    <div className="flex items-center justify-between">
                      <div className="text-sm">
                        Are you sure you want to delete{' '}
                        <span className="font-semibold">{m.id}</span>?
                      </div>
                      <div className="flex gap-2">
                        <Button variant="ghost" onClick={() => setDeleteConfirm(null)}>
                          Cancel
                        </Button>
                        <Button
                          variant="destructive"
                          onClick={() => deleteMutation.mutate(m.id)}
                          disabled={deleteMutation.isPending}
                        >
                          {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <div>
                      <div className={`flex items-center justify-between${!m.enabled ? ' opacity-50' : ''}`}>
                        <div className="flex items-center gap-3 min-w-0">
                          <Cpu className="h-5 w-5 text-muted-foreground shrink-0" />
                          <div className="min-w-0">
                            <div className="flex items-center gap-2">
                              <span className="font-medium">{m.id}</span>
                              <Badge className={`text-xs ${cliTypeBadgeColor(m.cli_type)}`}>
                                {m.cli_type}
                              </Badge>
                              {m.read_only && (
                                <Badge variant="secondary" className="text-xs">
                                  <Lock className="h-3 w-3 mr-1" />
                                  Built-in
                                </Badge>
                              )}
                            </div>
                            <div className="text-sm text-muted-foreground">
                              {m.display_name} &middot; {m.mapped_model} &middot; {m.context_length.toLocaleString()} ctx
                            </div>
                          </div>
                        </div>
                        <div className="flex gap-2 shrink-0 items-center">
                          <Toggle
                            checked={m.enabled}
                            disabled={m.read_only || toggleMutation.isPending}
                            onChange={() => toggleMutation.mutate({ id: m.id, enabled: !m.enabled })}
                          />
                          <div className="flex gap-1 items-center relative">
                            <CLIModelCheckButton
                              modelId={m.id}
                              disabled={editingId !== null || deleteConfirm !== null}
                            />
                            {!m.read_only && (
                              <>
                                <Button variant="ghost" size="icon" onClick={() => handleStartEdit(m)}>
                                  <Pencil className="h-4 w-4" />
                                </Button>
                                <Button variant="ghost" size="icon" onClick={() => setDeleteConfirm(m.id)}>
                                  <Trash2 className="h-4 w-4" />
                                </Button>
                              </>
                            )}
                          </div>
                        </div>
                      </div>
                      {toggleErrors[m.id] && (
                        <p className="text-sm text-destructive mt-1">{toggleErrors[m.id]}</p>
                      )}
                    </div>
                  )}
                </div>
              ))}
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}
