import { useState } from 'react'
import { ChevronDown, ChevronRight, Pencil, Trash2, Download } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { AgentDefsSection } from '@/components/workflow/AgentDefsSection'
import { WorkflowNotificationsSection } from '@/components/workflows/WorkflowNotificationsSection'
import type { WorkflowDefSummary } from '@/types/workflow'
import { cn } from '@/lib/utils'

function GroupBadges({ groups }: { groups?: string[] }) {
  if (!groups?.length) return null
  return (
    <>
      {groups.map((g) => (
        <Badge key={g} variant="outline" className="text-xs border-emerald-300 text-emerald-600">
          {g}
        </Badge>
      ))}
    </>
  )
}

export function WorkflowCard({
  id,
  def,
  onEdit,
  onDelete,
  onExport,
}: {
  id: string
  def: WorkflowDefSummary
  onEdit: () => void
  onDelete: () => void
  onExport: () => void
}) {
  const [expanded, setExpanded] = useState(false)

  // Group agents by layer for display
  const phases = (() => {
    if (!def.phases?.length) return ''
    const byLayer: Record<number, string[]> = {}
    for (const p of def.phases) {
      const layer = p.layer ?? 0
      if (!byLayer[layer]) byLayer[layer] = []
      byLayer[layer].push(p.agent || p.id)
    }
    return Object.keys(byLayer)
      .map(Number)
      .sort((a, b) => a - b)
      .map((l) => {
        if (byLayer[l].length > 1) {
          const policy = def.layer_policies?.[l]
          const suffix = policy ? `·${policy}` : ''
          return `[${byLayer[l].join(' | ')}]${suffix}`
        }
        return byLayer[l][0]
      })
      .join(' -> ')
  })()

  return (
    <div className="border border-border rounded-lg overflow-hidden">
      <div
        className={cn(
          'w-full p-4 flex items-center justify-between text-left hover:bg-muted/30 transition-colors',
          expanded && 'border-b border-border'
        )}
      >
        <button
          className="flex items-center gap-3 flex-1 text-left"
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? (
            <ChevronDown className="h-4 w-4 text-muted-foreground" />
          ) : (
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          )}
          <div>
            <div className="font-medium">{id}</div>
            {def.description && (
              <p className="text-xs text-muted-foreground mt-0.5">{def.description}</p>
            )}
          </div>
        </button>
        <div className="flex items-center gap-2">
          {def.scope_type === 'project' && (
            <Badge variant="outline" className="text-xs border-blue-300 text-blue-600">
              project
            </Badge>
          )}
          <GroupBadges groups={def.groups} />
          <span className="text-xs text-muted-foreground">
            {def.phases?.length || 0} agents
          </span>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0"
            onClick={onExport}
            title="Export workflow"
          >
            <Download className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0"
            onClick={onEdit}
            title="Edit workflow"
          >
            <Pencil className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0 text-destructive hover:text-destructive"
            onClick={onDelete}
            title="Delete workflow"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {expanded && (
        <div className="p-4 space-y-4">
          {phases && (
            <div>
              <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-1">
                Agent Pipeline
              </h4>
              <p className="text-sm font-mono">{phases}</p>
            </div>
          )}

          <AgentDefsSection workflowId={id} groups={def.groups || []} />
          <WorkflowNotificationsSection workflowId={id} />
        </div>
      )}
    </div>
  )
}
