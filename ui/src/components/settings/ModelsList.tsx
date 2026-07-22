import { useState } from 'react'
import { Cpu, Lock, Pencil, Plus, Trash2 } from 'lucide-react'
import type { Model } from '@/api/models'
import type { CustomProvider, UpdateCustomProviderRequest } from '@/api/customProviders'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/Card'
import { Toggle } from '@/components/ui/Toggle'
import { useCreateModel, useDeleteModel, useModels, useUpdateModel } from '@/hooks/useModels'
import { useCustomProviders, useUpdateCustomProvider, useDeleteCustomProvider } from '@/hooks/useCustomProviders'
import { CLIModelCheckButton } from './CLIModelCheckButton'
import { ModelForm } from './ModelForm'
import { emptyModelForm, modelToFormData, type ModelFormData } from './modelFormData'
import { BUILTIN_PROVIDER_IDS } from './ModelsProviderTabs'
import { CustomProviderForm, providerToFormData, type CustomProviderFormData } from './CustomProviderForm'

interface Props { provider: string }

function parseContext(value: string) {
  const parsed = Number.parseInt(value, 10)
  return Number.isNaN(parsed) ? 200000 : parsed
}

function requestFromForm(data: ModelFormData) {
  return {
    provider: data.provider,
    display_name: data.display_name.trim(),
    cli_model: data.cli_model.trim(),
    api_model: data.api_model.trim(),
    cli_efforts: data.cli_model ? data.cli_efforts : [],
    api_efforts: data.api_model ? data.api_efforts : [],
    cli_context: parseContext(data.cli_context),
    api_context: parseContext(data.api_context),
    fallback_models: data.fallback_models.trim(),
    default_effort: data.default_effort,
  }
}

export function ModelsList({ provider }: Props) {
  const { data: allModels = [], isLoading, error } = useModels()
  const { data: customProviders = [] } = useCustomProviders()
  const createMutation = useCreateModel()
  const updateMutation = useUpdateModel()
  const deleteMutation = useDeleteModel()
  const models = allModels.filter((model) => model.provider === provider)
  const isBuiltin = BUILTIN_PROVIDER_IDS.has(provider)
  const customProvider = isBuiltin ? undefined : customProviders.find((p) => p.name === provider)
  const apiOnly = provider === 'openrouter' || !isBuiltin
  const [creating, setCreating] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const [formData, setFormData] = useState<ModelFormData>(emptyModelForm)
  const [toggleErrors, setToggleErrors] = useState<Record<string, string>>({})

  const reset = () => {
    setCreating(false)
    setEditingId(null)
    setFormData(emptyModelForm)
  }
  const startCreate = () => {
    setCreating(true)
    setEditingId(null)
    setFormData({ ...emptyModelForm, provider })
  }
  const startEdit = (model: Model) => {
    setCreating(false)
    setEditingId(model.id)
    setFormData(modelToFormData(model))
  }
  const saveCreate = () => createMutation.mutate(
    { id: formData.id.trim(), ...requestFromForm(formData) },
    { onSuccess: reset },
  )
  const saveEdit = () => {
    if (!editingId) return
    const model = models.find((item) => item.id === editingId)
    const request = requestFromForm(formData)
    const editableFields = {
      display_name: request.display_name, cli_model: request.cli_model, api_model: request.api_model,
      cli_efforts: request.cli_efforts, api_efforts: request.api_efforts,
      cli_context: request.cli_context, api_context: request.api_context,
      fallback_models: request.fallback_models, default_effort: request.default_effort,
    }
    const data = model?.read_only
      ? { default_effort: formData.default_effort, fallback_models: formData.fallback_models.trim() }
      : editableFields
    updateMutation.mutate({ id: editingId, data }, { onSuccess: reset })
  }
  const toggle = (model: Model) => updateMutation.mutate(
    { id: model.id, data: { enabled: !model.enabled } },
    {
      onSuccess: () => setToggleErrors((current) => {
        const next = { ...current }; delete next[model.id]; return next
      }),
      onError: (cause) => setToggleErrors((current) => ({ ...current, [model.id]: cause.message })),
    },
  )

  return (
    <div className="space-y-4">
      {customProvider && <CustomProviderCard provider={customProvider} disabled={creating || editingId !== null || deleteId !== null} />}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Models</CardTitle>
              <CardDescription>Manage CLI and direct API modes in one model registry</CardDescription>
            </div>
            <Button onClick={startCreate} disabled={creating}><Plus className="mr-2 h-4 w-4" />New Model</Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          {isLoading && <div className="py-8 text-center text-muted-foreground">Loading models...</div>}
          {error && <div className="py-8 text-center text-destructive">Error: {error.message}</div>}
          {creating && <ModelForm formData={formData} setFormData={setFormData} onCancel={reset} onSave={saveCreate} mutation={createMutation} isCreate apiOnly={apiOnly} />}
          {!isLoading && !error && models.length === 0 && !creating && (
            <div className="py-8 text-center text-muted-foreground">No models found. Create one to get started.</div>
          )}
          {models.map((model) => (
            <div key={model.id} className="rounded-lg border p-4">
              {editingId === model.id ? (
                <ModelForm formData={formData} setFormData={setFormData} onCancel={reset} onSave={saveEdit} mutation={updateMutation} readOnly={model.read_only} apiOnly={apiOnly} />
              ) : deleteId === model.id ? (
                <DeleteConfirm model={model} pending={deleteMutation.isPending} onCancel={() => setDeleteId(null)}
                  onDelete={() => deleteMutation.mutate(model.id, { onSuccess: () => setDeleteId(null) })} />
              ) : (
                <div>
                  <div className={`flex items-center justify-between ${model.enabled ? '' : 'opacity-50'}`}>
                    <div className="flex min-w-0 items-center gap-3">
                      <Cpu className="h-5 w-5 shrink-0 text-muted-foreground" />
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-medium">{model.id}</span>
                          {model.cli_model && <Badge variant="secondary">CLI ✓</Badge>}
                          {model.api_model && <Badge variant="secondary">API ✓</Badge>}
                          {model.read_only && <Badge variant="secondary"><Lock className="mr-1 h-3 w-3" />Built-in</Badge>}
                        </div>
                        <div className="text-sm text-muted-foreground">{model.display_name}</div>
                      </div>
                    </div>
                    <div className="flex shrink-0 items-center gap-1">
                      <Toggle checked={model.enabled} disabled={model.read_only || updateMutation.isPending} onChange={() => toggle(model)} />
                      {model.cli_model && <CLIModelCheckButton modelId={model.id} disabled={editingId !== null || deleteId !== null} />}
                      <Button variant="ghost" size="icon" onClick={() => startEdit(model)}><Pencil className="h-4 w-4" /></Button>
                      {!model.read_only && <Button variant="ghost" size="icon" onClick={() => setDeleteId(model.id)}><Trash2 className="h-4 w-4" /></Button>}
                    </div>
                  </div>
                  {toggleErrors[model.id] && <p className="mt-1 text-sm text-destructive">{toggleErrors[model.id]}</p>}
                </div>
              )}
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  )
}

function CustomProviderCard({ provider, disabled }: { provider: CustomProvider; disabled: boolean }) {
  const updateMutation = useUpdateCustomProvider()
  const deleteMutation = useDeleteCustomProvider()
  const [editing, setEditing] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [formData, setFormData] = useState<CustomProviderFormData>(() => providerToFormData(provider))
  const [deleteError, setDeleteError] = useState('')

  const startEdit = () => { setFormData(providerToFormData(provider)); setEditing(true) }
  const saveEdit = () => {
    const data: UpdateCustomProviderRequest = { base_url: formData.base_url.trim(), api_wire: formData.api_wire, enabled: formData.enabled }
    if (formData.api_key.trim()) data.api_key = formData.api_key.trim()
    updateMutation.mutate({ name: provider.name, data }, { onSuccess: () => setEditing(false) })
  }
  const remove = () => deleteMutation.mutate(provider.name, {
    onError: (cause) => setDeleteError(cause.message),
  })

  if (editing) {
    return (
      <Card>
        <CardContent className="pt-6">
          <CustomProviderForm formData={formData} setFormData={setFormData} onCancel={() => setEditing(false)} onSave={saveEdit} mutation={updateMutation} existingApiKey={provider.api_key} />
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>{provider.name}</CardTitle>
            <CardDescription>{provider.base_url} · {provider.api_wire === 'chat_completions' ? 'Chat Completions API' : provider.api_wire === 'ollama_native' ? 'Ollama Native' : 'Responses API'}</CardDescription>
          </div>
          <div className="flex items-center gap-1">
            {!provider.enabled && <Badge variant="secondary">Disabled</Badge>}
            <Button variant="ghost" size="icon" disabled={disabled} onClick={startEdit}><Pencil className="h-4 w-4" /></Button>
            <Button variant="ghost" size="icon" disabled={disabled} onClick={() => { setDeleteError(''); setDeleting(true) }}><Trash2 className="h-4 w-4" /></Button>
          </div>
        </div>
      </CardHeader>
      {deleting && (
        <CardContent className="space-y-2">
          <div className="flex items-center justify-between text-sm">
            <span>Delete provider <strong>{provider.name}</strong>? Models referencing it must be removed first.</span>
            <div className="flex gap-2">
              <Button variant="ghost" onClick={() => setDeleting(false)}>Cancel</Button>
              <Button variant="destructive" disabled={deleteMutation.isPending} onClick={remove}>
                {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
              </Button>
            </div>
          </div>
          {deleteError && <p className="text-sm text-destructive">{deleteError}</p>}
        </CardContent>
      )}
    </Card>
  )
}

function DeleteConfirm({ model, pending, onCancel, onDelete }: { model: Model; pending: boolean; onCancel: () => void; onDelete: () => void }) {
  return (
    <div className="flex items-center justify-between text-sm">
      <span>Are you sure you want to delete <strong>{model.id}</strong>?</span>
      <div className="flex gap-2">
        <Button variant="ghost" onClick={onCancel}>Cancel</Button>
        <Button variant="destructive" disabled={pending} onClick={onDelete}>{pending ? 'Deleting...' : 'Delete'}</Button>
      </div>
    </div>
  )
}
