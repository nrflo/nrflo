import { useState } from 'react'
import { Plus, Pencil, Trash2, KeyRound } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/Card'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { APICredentialForm } from '@/components/apiCredentials/APICredentialForm'
import { ReadOnlyHint } from '@/components/auth/ReadOnlyHint'
import {
  useAPICredentials,
  useCreateAPICredential,
  useUpdateAPICredential,
  useDeleteAPICredential,
} from '@/hooks/useAPICredentials'
import { useProjects } from '@/hooks/useProjects'
import { useIsAdmin } from '@/stores/authStore'
import type { APICredential, APICredentialCreateRequest, APICredentialUpdateRequest } from '@/types/apiCredential'

function redactSecretRef(ref: string): string {
  if (ref.startsWith('literal:')) return 'literal:***'
  return ref
}

export function APICredentialsPage() {
  const [isCreating, setIsCreating] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null)
  const isAdmin = useIsAdmin()

  const { data: credentials = [], isLoading, error } = useAPICredentials()
  const { data: projectsData } = useProjects()
  const projects = projectsData?.projects ?? []
  const createMutation = useCreateAPICredential()
  const updateMutation = useUpdateAPICredential()
  const deleteMutation = useDeleteAPICredential()

  const projectName = (id?: string) => {
    if (!id) return 'Global'
    return projects.find((p) => p.id === id)?.name ?? id
  }

  const handleCreate = (data: APICredentialCreateRequest | APICredentialUpdateRequest) => {
    createMutation.mutate(data as APICredentialCreateRequest, {
      onSuccess: () => setIsCreating(false),
    })
  }

  const handleUpdate = (id: string) => (data: APICredentialCreateRequest | APICredentialUpdateRequest) => {
    const req = data as APICredentialUpdateRequest
    if (!req.secret_ref?.trim()) delete req.secret_ref
    updateMutation.mutate({ id, data: req }, {
      onSuccess: () => setEditingId(null),
    })
  }

  const handleDelete = (id: string) => {
    deleteMutation.mutate(id, {
      onSuccess: () => setDeleteConfirmId(null),
    })
  }

  return (
    <div className="p-6 max-w-4xl mx-auto space-y-4">
      {!isAdmin && <ReadOnlyHint />}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <KeyRound className="h-5 w-5" />
                API Credentials
              </CardTitle>
              <CardDescription>Provider API keys for API-mode agents</CardDescription>
            </div>
            {isAdmin && (
              <Button onClick={() => { setIsCreating(true); setEditingId(null) }} disabled={isCreating}>
                <Plus className="h-4 w-4 mr-2" />
                New Credential
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {isLoading && <p className="text-center py-8 text-muted-foreground">Loading…</p>}
            {error && <p className="text-center py-8 text-destructive">Error: {(error as Error).message}</p>}
            {isAdmin && isCreating && (
              <APICredentialForm
                isCreate
                projects={projects}
                onSubmit={handleCreate}
                onCancel={() => setIsCreating(false)}
                isPending={createMutation.isPending}
              />
            )}
            {!isLoading && !error && credentials.length === 0 && !isCreating && (
              <p className="text-center py-8 text-muted-foreground">No API credentials configured yet.</p>
            )}
            {credentials.map((cred: APICredential) => (
              <div key={cred.id} className="border rounded-lg p-4">
                {isAdmin && editingId === cred.id ? (
                  <APICredentialForm
                    initial={cred}
                    isCreate={false}
                    projects={projects}
                    onSubmit={handleUpdate(cred.id)}
                    onCancel={() => setEditingId(null)}
                    isPending={updateMutation.isPending}
                  />
                ) : (
                  <div className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-medium text-sm capitalize">{cred.provider}</span>
                        <span className="text-xs text-muted-foreground px-1.5 py-0.5 rounded bg-muted">
                          {projectName(cred.project_id ?? undefined)}
                        </span>
                      </div>
                      <div className="text-xs text-muted-foreground font-mono mt-0.5">
                        {redactSecretRef(cred.secret_ref)}
                      </div>
                    </div>
                    {isAdmin && (
                      <div className="flex gap-1 shrink-0">
                        <Button variant="ghost" size="icon" onClick={() => { setEditingId(cred.id); setIsCreating(false) }}>
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button variant="ghost" size="icon" onClick={() => setDeleteConfirmId(cred.id)}>
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
      <ConfirmDialog
        open={!!deleteConfirmId}
        onClose={() => setDeleteConfirmId(null)}
        onConfirm={() => deleteConfirmId && handleDelete(deleteConfirmId)}
        title="Delete API Credential"
        message={`Delete credential '${deleteConfirmId}'?`}
        confirmLabel="Delete"
        variant="destructive"
      />
    </div>
  )
}
