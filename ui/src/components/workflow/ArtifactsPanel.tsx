import { useState } from 'react'
import { Download, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { useArtifacts, useDeleteArtifact } from '@/hooks/useArtifacts'
import { downloadArtifactURL } from '@/api/artifacts'
import type { Artifact } from '@/types/artifact'

interface ArtifactsPanelProps {
  workflowInstanceId: string | undefined
  sessionId?: string
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function SourceBadge({ source }: { source: 'input' | 'agent' }) {
  const cls = source === 'input'
    ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
    : 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
  return (
    <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-semibold shrink-0 ${cls}`}>
      {source}
    </span>
  )
}

function ArtifactRow({ artifact, workflowInstanceId }: { artifact: Artifact; workflowInstanceId: string }) {
  const [confirmOpen, setConfirmOpen] = useState(false)
  const deleteMutation = useDeleteArtifact()

  return (
    <div className="flex items-center gap-2 border-b border-border/50 last:border-b-0 px-2 py-1.5">
      <span className="flex-1 text-xs truncate">{artifact.name}</span>
      <span className="text-xs text-muted-foreground shrink-0">{formatBytes(artifact.size_bytes)}</span>
      <SourceBadge source={artifact.source} />
      <a href={downloadArtifactURL(artifact.id)} download={artifact.name} className="shrink-0">
        <Button variant="ghost" size="sm" className="h-6 w-6 p-0">
          <Download className="h-3 w-3" />
        </Button>
      </a>
      <Button
        variant="ghost"
        size="sm"
        className="h-6 w-6 p-0 text-destructive hover:text-destructive shrink-0"
        onClick={() => setConfirmOpen(true)}
        disabled={deleteMutation.isPending}
      >
        <Trash2 className="h-3 w-3" />
      </Button>
      <ConfirmDialog
        open={confirmOpen}
        onClose={() => setConfirmOpen(false)}
        onConfirm={() => deleteMutation.mutate({ id: artifact.id, workflowInstanceId })}
        title="Delete Artifact"
        message={`Delete "${artifact.name}"? This cannot be undone.`}
        confirmLabel="Delete"
        variant="destructive"
      />
    </div>
  )
}

export function ArtifactsPanel({ workflowInstanceId, sessionId }: ArtifactsPanelProps) {
  const { data: artifacts = [] } = useArtifacts(workflowInstanceId)

  const filtered = sessionId
    ? artifacts.filter(a => a.source === 'input' || a.created_by_session === sessionId)
    : artifacts.filter(a => a.source === 'input')

  if (filtered.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
        <p className="text-xs">No artifacts available</p>
      </div>
    )
  }

  return (
    <div className="border border-border rounded">
      {filtered.map(artifact => (
        <ArtifactRow key={artifact.id} artifact={artifact} workflowInstanceId={workflowInstanceId!} />
      ))}
    </div>
  )
}
