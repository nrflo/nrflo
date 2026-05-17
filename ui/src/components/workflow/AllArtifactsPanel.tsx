import { Download } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { useArtifacts } from '@/hooks/useArtifacts'
import { downloadArtifactURL } from '@/api/artifacts'
import type { Artifact } from '@/types/artifact'

interface AllArtifactsPanelProps {
  workflowInstanceId: string | undefined
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function ArtifactRow({ artifact }: { artifact: Artifact }) {
  return (
    <div className="flex items-center gap-2 border-b border-border/50 last:border-b-0 px-2 py-1.5">
      <span className="flex-1 text-xs truncate">{artifact.name}</span>
      <span className="text-xs text-muted-foreground shrink-0">{formatBytes(artifact.size_bytes)}</span>
      <a href={downloadArtifactURL(artifact.id)} download={artifact.name} className="shrink-0">
        <Button variant="ghost" size="sm" className="h-6 w-6 p-0">
          <Download className="h-3 w-3" />
        </Button>
      </a>
    </div>
  )
}

export function AllArtifactsPanel({ workflowInstanceId }: AllArtifactsPanelProps) {
  const { data: artifacts = [] } = useArtifacts(workflowInstanceId)

  if (artifacts.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
        <p className="text-xs">No artifacts available</p>
      </div>
    )
  }

  const inputArtifacts = artifacts.filter(a => a.source === 'input')
  const agentArtifacts = artifacts.filter(a => a.source === 'agent')

  const bySession = agentArtifacts.reduce<Record<string, Artifact[]>>((acc, a) => {
    const key = a.created_by_session ?? 'unknown'
    if (!acc[key]) acc[key] = []
    acc[key].push(a)
    return acc
  }, {})

  return (
    <div className="space-y-3">
      {inputArtifacts.length > 0 && (
        <div>
          <h4 className="text-sm font-medium text-foreground px-2 mb-1">Input Artifacts</h4>
          <div className="border border-border rounded">
            {inputArtifacts.map(a => <ArtifactRow key={a.id} artifact={a} />)}
          </div>
        </div>
      )}
      {Object.entries(bySession).map(([sessionId, sessionArtifacts]) => (
        <div key={sessionId}>
          <h4 className="text-sm font-medium text-foreground px-2 mb-1">
            Agent: <span className="font-mono text-xs">{sessionId}</span>
          </h4>
          <div className="border border-border rounded">
            {sessionArtifacts.map(a => <ArtifactRow key={a.id} artifact={a} />)}
          </div>
        </div>
      ))}
    </div>
  )
}
