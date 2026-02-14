import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Select } from '@/components/ui/Select'
import { Spinner } from '@/components/ui/Spinner'
import { CreateChainDialog } from '@/components/chains/CreateChainDialog'
import { useChainList } from '@/hooks/useChains'
import { cn, statusColor, formatRelativeTime, capitalize } from '@/lib/utils'
import type { ChainExecution } from '@/types/chain'

function ChainProgress({ chain }: { chain: ChainExecution }) {
  if (chain.total_items === 0) return null
  const pct = Math.round((chain.completed_items / chain.total_items) * 100)
  return (
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
  )
}

export function ChainListPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const [showCreate, setShowCreate] = useState(false)
  const statusFilter = searchParams.get('status') || ''

  const { data: chains, isLoading, error } = useChainList(
    statusFilter ? { status: statusFilter } : undefined,
    { refetchInterval: 5000 }
  )

  const handleStatusChange = (value: string) => {
    const newParams = new URLSearchParams(searchParams)
    if (value) {
      newParams.set('status', value)
    } else {
      newParams.delete('status')
    }
    setSearchParams(newParams)
  }

  return (
    <div className="space-y-6">
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
        <Select
          value={statusFilter}
          onChange={(e) => handleStatusChange(e.target.value)}
          className="w-40"
        >
          <option value="">All Statuses</option>
          <option value="pending">Pending</option>
          <option value="running">Running</option>
          <option value="completed">Completed</option>
          <option value="failed">Failed</option>
          <option value="canceled">Canceled</option>
        </Select>
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
        <div className="space-y-2">
          {chains.map((chain) => (
            <Link
              key={chain.id}
              to={`/chains/${chain.id}`}
              className="block border border-border rounded-lg p-4 hover:bg-muted/50 transition-colors"
            >
              <div className="flex items-center justify-between">
                <div className="space-y-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-medium truncate">{chain.name}</span>
                    <Badge className={statusColor(chain.status)}>
                      {capitalize(chain.status)}
                    </Badge>
                  </div>
                  <div className="flex items-center gap-3 text-xs text-muted-foreground">
                    <span>Workflow: {chain.workflow_name}</span>
                    <span>{formatRelativeTime(chain.created_at)}</span>
                  </div>
                </div>
                <ChainProgress chain={chain} />
              </div>
            </Link>
          ))}
        </div>
      )}

      <CreateChainDialog open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  )
}
