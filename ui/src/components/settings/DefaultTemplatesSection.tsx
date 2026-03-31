import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Pencil, Trash2, FileText, Lock } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/Card'
import { Badge } from '@/components/ui/Badge'
import {
  listDefaultTemplates,
  createDefaultTemplate,
  updateDefaultTemplate,
  deleteDefaultTemplate,
  type DefaultTemplate,
  type CreateDefaultTemplateRequest,
  type UpdateDefaultTemplateRequest,
} from '@/api/defaultTemplates'
import { DefaultTemplateForm, emptyTemplateForm, type TemplateFormData } from './DefaultTemplateForm'

const templateKeys = {
  all: ['default-templates'] as const,
  list: () => [...templateKeys.all, 'list'] as const,
}

function templateToFormData(t: DefaultTemplate): TemplateFormData {
  return { id: t.id, name: t.name, template: t.template }
}

export function DefaultTemplatesSection() {
  const queryClient = useQueryClient()

  const [isCreating, setIsCreating] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [formData, setFormData] = useState<TemplateFormData>(emptyTemplateForm)
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null)

  const { data: templates = [], isLoading, error } = useQuery({
    queryKey: templateKeys.list(),
    queryFn: listDefaultTemplates,
  })

  const createMutation = useMutation({
    mutationFn: (data: CreateDefaultTemplateRequest) => createDefaultTemplate(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: templateKeys.list() })
      setIsCreating(false)
      setFormData(emptyTemplateForm)
    },
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateDefaultTemplateRequest }) =>
      updateDefaultTemplate(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: templateKeys.list() })
      setEditingId(null)
      setFormData(emptyTemplateForm)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteDefaultTemplate(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: templateKeys.list() })
      setDeleteConfirm(null)
    },
  })

  const handleStartCreate = () => {
    setIsCreating(true)
    setEditingId(null)
    setFormData(emptyTemplateForm)
  }

  const handleStartEdit = (t: DefaultTemplate) => {
    setEditingId(t.id)
    setIsCreating(false)
    setFormData(templateToFormData(t))
  }

  const handleCancel = () => {
    setIsCreating(false)
    setEditingId(null)
    setFormData(emptyTemplateForm)
  }

  const handleSaveCreate = () => {
    if (!formData.id.trim() || !formData.name.trim() || !formData.template.trim()) return
    createMutation.mutate({
      id: formData.id.trim(),
      name: formData.name.trim(),
      template: formData.template,
    })
  }

  const handleSaveEdit = () => {
    if (!editingId) return
    updateMutation.mutate({
      id: editingId,
      data: { name: formData.name.trim(), template: formData.template },
    })
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Default Templates</CardTitle>
            <CardDescription>Global agent prompt templates</CardDescription>
          </div>
          <Button onClick={handleStartCreate} disabled={isCreating}>
            <Plus className="h-4 w-4 mr-2" />
            New Template
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">
          {isLoading && (
            <div className="text-center py-8 text-muted-foreground">Loading templates...</div>
          )}
          {error && (
            <div className="text-center py-8 text-destructive">
              Error: {(error as Error).message}
            </div>
          )}

          {isCreating && (
            <DefaultTemplateForm
              formData={formData}
              setFormData={setFormData}
              onCancel={handleCancel}
              onSave={handleSaveCreate}
              mutation={createMutation}
              isCreate
            />
          )}

          {!isLoading && !error && templates.length === 0 && !isCreating && (
            <div className="text-center py-8 text-muted-foreground">
              No default templates found. Create one to get started.
            </div>
          )}

          {templates.map((t) => (
            <div key={t.id} className="border rounded-lg p-4">
              {editingId === t.id ? (
                <DefaultTemplateForm
                  formData={formData}
                  setFormData={setFormData}
                  onCancel={handleCancel}
                  onSave={handleSaveEdit}
                  mutation={updateMutation}
                  isReadonly={t.readonly}
                />
              ) : deleteConfirm === t.id ? (
                <div className="flex items-center justify-between">
                  <div className="text-sm">
                    Are you sure you want to delete{' '}
                    <span className="font-semibold">{t.id}</span>?
                  </div>
                  <div className="flex gap-2">
                    <Button variant="ghost" onClick={() => setDeleteConfirm(null)}>
                      Cancel
                    </Button>
                    <Button
                      variant="destructive"
                      onClick={() => deleteMutation.mutate(t.id)}
                      disabled={deleteMutation.isPending}
                    >
                      {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
                    </Button>
                  </div>
                </div>
              ) : (
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3 min-w-0">
                    <FileText className="h-5 w-5 text-muted-foreground shrink-0" />
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-medium">{t.id}</span>
                        {t.readonly && (
                          <Badge variant="secondary" className="text-xs">
                            <Lock className="h-3 w-3 mr-1" />
                            Built-in
                          </Badge>
                        )}
                      </div>
                      <div className="text-sm text-muted-foreground">{t.name}</div>
                      <div className="text-xs text-muted-foreground mt-1 truncate max-w-xl font-mono">
                        {t.template.split('\n').slice(0, 2).join(' ').slice(0, 120)}
                        {t.template.length > 120 && '…'}
                      </div>
                    </div>
                  </div>
                  <div className="flex gap-1 shrink-0">
                    <Button variant="ghost" size="icon" onClick={() => handleStartEdit(t)}>
                      <Pencil className="h-4 w-4" />
                    </Button>
                    {!t.readonly && (
                      <Button variant="ghost" size="icon" onClick={() => setDeleteConfirm(t.id)}>
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    )}
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}
