import { useMemo, useState } from 'react'
import { Plus, Trash2, KeyRound } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { Spinner } from '@/components/ui/Spinner'
import { useProjectStore } from '@/stores/projectStore'
import { formatRelativeTime } from '@/lib/utils'
import { useServiceTokens, useDeleteServiceToken } from '@/hooks/useServiceTokens'
import { CreateServiceTokenDialog, RevealServiceTokenDialog } from './ServiceTokensSectionDialogs'
import type { ServiceToken } from '@/types/serviceToken'

const ALL_PROJECTS = '__all__'

export function ServiceTokensSection() {
  const projects = useProjectStore((s) => s.projects)
  const [filterProject, setFilterProject] = useState<string>(ALL_PROJECTS)
  const [showCreate, setShowCreate] = useState(false)
  const [revealed, setRevealed] = useState<{ token: string; record: ServiceToken } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ServiceToken | null>(null)

  const { data, isLoading, error } = useServiceTokens()
  const tokens = data ?? []
  const deleteMutation = useDeleteServiceToken()

  const projectNameById = useMemo(() => {
    const m = new Map<string, string>()
    for (const p of projects) m.set(p.id.toLowerCase(), p.name)
    return m
  }, [projects])

  const projectOptions = useMemo(() => {
    const opts = [{ value: ALL_PROJECTS, label: 'All projects' }]
    for (const p of projects) opts.push({ value: p.id, label: p.name })
    return opts
  }, [projects])

  const filtered = useMemo(() => {
    if (filterProject === ALL_PROJECTS) return tokens
    return tokens.filter((t) => t.project_id.toLowerCase() === filterProject.toLowerCase())
  }, [tokens, filterProject])

  const handleDelete = async () => {
    if (!deleteTarget) return
    try {
      await deleteMutation.mutateAsync(deleteTarget.id)
      setDeleteTarget(null)
    } catch {
      // surfaced through the disabled state; keep dialog open
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold">Service Tokens</h2>
          <p className="text-sm text-muted-foreground">
            Long-lived bearer tokens for external services to call the nrflo REST API. Each token
            is scoped to one project. {tokens.length} token{tokens.length === 1 ? '' : 's'}.
          </p>
        </div>
        <Button onClick={() => setShowCreate(true)} disabled={projects.length === 0}>
          <Plus className="h-4 w-4 mr-2" />
          Create Token
        </Button>
      </div>

      <div className="flex items-center gap-3">
        <span className="text-xs font-medium text-muted-foreground">Project</span>
        <div className="min-w-48">
          <Dropdown value={filterProject} onChange={setFilterProject} options={projectOptions} />
        </div>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : error ? (
        <p className="text-destructive text-sm">
          {error instanceof Error ? error.message : 'Failed to load service tokens'}
        </p>
      ) : filtered.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-12 text-muted-foreground">
          <KeyRound className="h-8 w-8 opacity-40" />
          <p className="text-sm">No service tokens yet.</p>
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Project</TableHead>
              <TableHead>Token</TableHead>
              <TableHead className="w-36">Created</TableHead>
              <TableHead className="w-36">Last Used</TableHead>
              <TableHead className="w-16" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.map((t) => (
              <TableRow key={t.id}>
                <TableCell className="font-medium">{t.name}</TableCell>
                <TableCell>{projectNameById.get(t.project_id.toLowerCase()) ?? t.project_id}</TableCell>
                <TableCell>
                  <code className="text-xs bg-muted px-1.5 py-0.5 rounded">{t.display_hint}</code>
                </TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {formatRelativeTime(t.created_at)}
                </TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {t.last_used_at ? formatRelativeTime(t.last_used_at) : 'Never'}
                </TableCell>
                <TableCell>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setDeleteTarget(t)}
                    aria-label={`Revoke token ${t.name}`}
                  >
                    <Trash2 className="h-4 w-4 text-destructive" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <CreateServiceTokenDialog
        open={showCreate}
        onClose={() => setShowCreate(false)}
        onCreated={(result) => {
          setShowCreate(false)
          setRevealed(result)
        }}
      />

      <RevealServiceTokenDialog
        open={revealed !== null}
        token={revealed?.token ?? ''}
        record={revealed?.record ?? null}
        onClose={() => setRevealed(null)}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        title="Revoke service token?"
        message={
          deleteTarget
            ? `The token "${deleteTarget.name}" will be revoked immediately. Any external service still using it will start getting 401 responses.`
            : ''
        }
        confirmLabel="Revoke"
        variant="destructive"
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
      />
    </div>
  )
}
