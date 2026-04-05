import { useState, useMemo } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Plus, ChevronLeft, ChevronRight, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Dropdown } from '@/components/ui/Dropdown'
import { Spinner } from '@/components/ui/Spinner'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { CreateChainDialog } from '@/components/chains/CreateChainDialog'
import { useChainList, useDeleteChain } from '@/hooks/useChains'
import { cn, statusColor, formatRelativeTime, formatElapsedTime, capitalize } from '@/lib/utils'

const PAGE_SIZE = 20

export function ChainListPage() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const [showCreate, setShowCreate] = useState(false)
  const [currentPage, setCurrentPage] = useState(0)
  const [deleteTargetId, setDeleteTargetId] = useState<string | null>(null)
  const statusFilter = searchParams.get('status') || ''

  const deleteChain = useDeleteChain()
  const { data: chains, isLoading, error } = useChainList(
    statusFilter ? { status: statusFilter } : undefined,
  )

  const pageCount = useMemo(() => Math.max(1, Math.ceil((chains?.length ?? 0) / PAGE_SIZE)), [chains?.length])
  const safePage = Math.min(currentPage, pageCount - 1)
  const pageItems = useMemo(
    () => chains?.slice(safePage * PAGE_SIZE, (safePage + 1) * PAGE_SIZE) ?? [],
    [chains, safePage],
  )

  const handleStatusChange = (value: string) => {
    const newParams = new URLSearchParams(searchParams)
    if (value) {
      newParams.set('status', value)
    } else {
      newParams.delete('status')
    }
    setSearchParams(newParams)
    setCurrentPage(0)
  }

  return (
    <div className="max-w-[85%] mx-auto space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Chain Executions</h1>
          <p className="text-muted-foreground">
            {chains?.length ?? 0} chain{chains?.length !== 1 ? 's' : ''}
          </p>
        </div>
        <Button onClick={() => setShowCreate(true)}>
          <Plus className="h-4 w-4 mr-2" />
          New Chain
        </Button>
      </div>

      <div className="flex items-center gap-4">
        <Dropdown
          value={statusFilter}
          onChange={handleStatusChange}
          className="w-40"
          options={[
            { value: '', label: 'All Statuses' },
            { value: 'pending', label: 'Pending' },
            { value: 'running', label: 'Running' },
            { value: 'completed', label: 'Completed' },
            { value: 'failed', label: 'Failed' },
            { value: 'canceled', label: 'Canceled' },
          ]}
        />
        {statusFilter && (
          <Button variant="ghost" size="sm" onClick={() => handleStatusChange('')}>
            Clear filter
          </Button>
        )}
      </div>

      {isLoading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : error ? (
        <p className="text-destructive text-sm">
          {error instanceof Error ? error.message : 'Failed to load chains'}
        </p>
      ) : !chains || chains.length === 0 ? (
        <div className="text-center py-12 text-muted-foreground">
          <p>No chains found. Create one to get started!</p>
        </div>
      ) : (
        <div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead className="w-32">Workflow</TableHead>
                <TableHead className="w-24">Status</TableHead>
                <TableHead className="w-40">Progress</TableHead>
                <TableHead className="w-28">Duration</TableHead>
                <TableHead className="w-28">Created By</TableHead>
                <TableHead className="w-24">Created</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {pageItems.map((chain) => {
                const pct = chain.total_items > 0
                  ? Math.round((chain.completed_items / chain.total_items) * 100)
                  : 0
                return (
                  <TableRow
                    key={chain.id}
                    onClick={() => navigate(`/chains/${chain.id}`)}
                    className="cursor-pointer"
                    data-testid="chain-row"
                  >
                    <TableCell className="font-medium">{chain.name}</TableCell>
                    <TableCell className="text-muted-foreground">{chain.workflow_name}</TableCell>
                    <TableCell>
                      <Badge className={statusColor(chain.status)}>
                        {capitalize(chain.status)}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      {chain.total_items > 0 && (
                        <div className="flex items-center gap-2 text-xs text-muted-foreground">
                          <div className="w-24 h-1.5 bg-muted rounded-full overflow-hidden">
                            <div
                              className={cn(
                                'h-full rounded-full transition-all',
                                chain.status === 'failed' ? 'bg-red-500' : 'bg-green-500'
                              )}
                              style={{ width: `${pct}%` }}
                            />
                          </div>
                          <span>{chain.completed_items}/{chain.total_items}</span>
                        </div>
                      )}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-xs">
                      {chain.started_at ? formatElapsedTime(chain.started_at, chain.completed_at) : '—'}
                    </TableCell>
                    <TableCell className="text-muted-foreground">{chain.created_by || '-'}</TableCell>
                    <TableCell className="text-muted-foreground">{formatRelativeTime(chain.created_at)}</TableCell>
                    <TableCell>
                      <button
                        onClick={(e) => { e.stopPropagation(); setDeleteTargetId(chain.id) }}
                        disabled={chain.status === 'running'}
                        className={cn(
                          'text-muted-foreground hover:text-destructive transition-colors',
                          chain.status === 'running' && 'opacity-50 cursor-not-allowed'
                        )}
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
                {safePage * PAGE_SIZE + 1}–{Math.min((safePage + 1) * PAGE_SIZE, chains.length)} of {chains.length}
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
      )}

      <CreateChainDialog open={showCreate} onClose={() => setShowCreate(false)} />
      <ConfirmDialog
        open={!!deleteTargetId}
        onClose={() => setDeleteTargetId(null)}
        onConfirm={() => {
          if (deleteTargetId) {
            deleteChain.mutate(deleteTargetId, { onSettled: () => setDeleteTargetId(null) })
          }
        }}
        title="Delete Chain"
        message={`Are you sure you want to delete this chain? This action cannot be undone.`}
        confirmLabel="Delete"
        variant="destructive"
      />
    </div>
  )
}
