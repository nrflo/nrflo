import { useState, useMemo } from 'react'
import { Trash2, ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { ResultIcon } from '@/components/ui/ResultIcon'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
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
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Workflow</TableHead>
            <TableHead className="w-24">Instance</TableHead>
            <TableHead className="w-16">Status</TableHead>
            <TableHead className="w-24">Duration</TableHead>
            <TableHead>Completed At</TableHead>
            <TableHead className="w-10"></TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {pageItems.map((id) => {
            const state = instances[id]
            const isSelected = id === selectedId
            const status = state?.status
            const isFailed = status === 'failed'

            return (
              <TableRow
                key={id}
                onClick={() => onSelect(id)}
                className={cn(
                  'cursor-pointer',
                  isSelected && 'bg-primary/10'
                )}
                data-testid="instance-row"
              >
                <TableCell>{state?.workflow ?? '-'}</TableCell>
                <TableCell className="text-muted-foreground">#{id.substring(0, 8)}</TableCell>
                <TableCell>
                  <ResultIcon result={isFailed ? 'fail' : 'pass'} className="text-[10px]" />
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {state?.total_duration_sec ? formatDurationSec(state.total_duration_sec) : '-'}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {state?.completed_at ? formatDateTime(state.completed_at) : '-'}
                </TableCell>
                <TableCell className="w-10">
                  <button
                    onClick={(e) => { e.stopPropagation(); onDelete(id) }}
                    className="text-muted-foreground hover:text-destructive transition-colors"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
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
