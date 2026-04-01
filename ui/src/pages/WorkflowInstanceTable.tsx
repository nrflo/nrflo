import { useState, useMemo } from 'react'
import { Trash2, ChevronLeft, ChevronRight } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { cn, formatDateTime, formatDurationSec } from '@/lib/utils'
import type { WorkflowState } from '@/types/workflow'

const PAGE_SIZE = 10

interface WorkflowInstanceTableProps {
  instanceIds: string[]
  instances: Record<string, WorkflowState>
  selectedId: string
  onSelect: (id: string) => void
  onDelete: (id: string) => void
}

export function WorkflowInstanceTable({
  instanceIds,
  instances,
  selectedId,
  onSelect,
  onDelete,
}: WorkflowInstanceTableProps) {
  const [currentPage, setCurrentPage] = useState(0)

  const pageCount = useMemo(() => Math.max(1, Math.ceil(instanceIds.length / PAGE_SIZE)), [instanceIds.length])
  const safePage = Math.min(currentPage, pageCount - 1)
  const pageItems = useMemo(() => instanceIds.slice(safePage * PAGE_SIZE, (safePage + 1) * PAGE_SIZE), [instanceIds, safePage])

  if (instanceIds.length === 0) return null

  return (
    <div>
      <table className="w-full text-xs font-mono border-collapse">
        <thead>
          <tr className="text-left text-muted-foreground border-b border-border">
            <th className="py-1.5 pr-3 font-medium">Workflow</th>
            <th className="py-1.5 pr-3 font-medium">Instance</th>
            <th className="py-1.5 pr-3 font-medium">Status</th>
            <th className="py-1.5 pr-3 font-medium">Duration</th>
            <th className="py-1.5 pr-3 font-medium">Completed At</th>
            <th className="py-1.5 font-medium w-10"></th>
          </tr>
        </thead>
        <tbody>
          {pageItems.map((id) => {
            const state = instances[id]
            const isSelected = id === selectedId
            const status = state?.status
            const isFailed = status === 'failed'

            return (
              <tr
                key={id}
                onClick={() => onSelect(id)}
                className={cn(
                  'border-b border-border/50 hover:bg-muted/50 cursor-pointer transition-colors',
                  isSelected && 'bg-primary/10'
                )}
              >
                <td className="py-1.5 pr-3">{state?.workflow ?? '-'}</td>
                <td className="py-1.5 pr-3 text-muted-foreground">#{id.substring(0, 8)}</td>
                <td className="py-1.5 pr-3">
                  <Badge
                    variant={isFailed ? 'destructive' : 'success'}
                    className="text-[10px] px-1 py-0"
                  >
                    {isFailed ? 'fail' : 'pass'}
                  </Badge>
                </td>
                <td className="py-1.5 pr-3 text-muted-foreground">
                  {state?.total_duration_sec ? formatDurationSec(state.total_duration_sec) : '-'}
                </td>
                <td className="py-1.5 pr-3 text-muted-foreground">
                  {state?.completed_at ? formatDateTime(state.completed_at) : '-'}
                </td>
                <td className="py-1.5">
                  <button
                    onClick={(e) => { e.stopPropagation(); onDelete(id) }}
                    className="text-muted-foreground hover:text-destructive transition-colors"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
      {pageCount > 1 && (
        <div className="flex items-center justify-between pt-3 text-xs text-muted-foreground">
          <span>
            {safePage * PAGE_SIZE + 1}–{Math.min((safePage + 1) * PAGE_SIZE, instanceIds.length)} of {instanceIds.length}
          </span>
          <div className="flex gap-1">
            <Button
              variant="outline"
              size="sm"
              disabled={safePage === 0}
              onClick={() => setCurrentPage(p => p - 1)}
              className="h-7 w-7 p-0"
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={safePage >= pageCount - 1}
              onClick={() => setCurrentPage(p => p + 1)}
              className="h-7 w-7 p-0"
            >
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
