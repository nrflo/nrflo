import { useState } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { cn, formatDateTime } from '@/lib/utils'
import { useErrors } from '@/hooks/useErrors'
import type { ErrorLog } from '@/types/errors'

const PAGE_SIZE = 20

const TYPE_FILTERS = [
  { id: '', label: 'All' },
  { id: 'agent', label: 'Agent' },
  { id: 'workflow', label: 'Workflow' },
  { id: 'system', label: 'System' },
] as const

function typeBadgeClass(errorType: ErrorLog['error_type']): string {
  switch (errorType) {
    case 'agent': return 'bg-red-500/15 text-red-600 dark:text-red-400'
    case 'workflow': return 'bg-yellow-500/15 text-yellow-600 dark:text-yellow-400'
    case 'system': return 'bg-blue-500/15 text-blue-600 dark:text-blue-400'
  }
}

export function ErrorsPage() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const activeType = searchParams.get('type') ?? ''
  const [page, setPage] = useState(1)

  const { data, isLoading } = useErrors({
    page,
    perPage: PAGE_SIZE,
    type: activeType || undefined,
  })

  const errors = data?.errors ?? []
  const total = data?.total ?? 0
  const totalPages = data?.total_pages ?? 1

  const handleTypeChange = (type: string) => {
    setPage(1)
    if (type) {
      setSearchParams({ type })
    } else {
      setSearchParams({})
    }
  }

  const handleRowClick = (_error: ErrorLog) => {
    navigate('/project-workflows')
  }

  const startItem = (page - 1) * PAGE_SIZE + 1
  const endItem = Math.min(page * PAGE_SIZE, total)

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-bold">Errors</h1>

      <div className="border-b border-border">
        <div className="flex gap-1">
          {TYPE_FILTERS.map(({ id, label }) => (
            <button
              key={id}
              onClick={() => handleTypeChange(id)}
              className={cn(
                'flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors',
                activeType === id
                  ? 'border-primary text-primary'
                  : 'border-transparent text-muted-foreground hover:text-foreground'
              )}
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      {isLoading ? (
        <div className="text-sm text-muted-foreground">Loading...</div>
      ) : errors.length === 0 ? (
        <div className="text-center text-muted-foreground py-12">
          No errors recorded
        </div>
      ) : (
        <div className="border rounded-lg overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/30">
                <TableHead className="w-24">Type</TableHead>
                <TableHead className="w-24">SID</TableHead>
                <TableHead className="w-28">Instance</TableHead>
                <TableHead>Message</TableHead>
                <TableHead className="w-40">Date/Time</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {errors.map((error) => (
                <TableRow
                  key={error.id}
                  onClick={() => handleRowClick(error)}
                  className="cursor-pointer font-mono text-xs"
                >
                  <TableCell>
                    <Badge className={typeBadgeClass(error.error_type)}>
                      {error.error_type}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    {error.error_type === 'agent' ? (
                      <button
                        className="text-primary hover:underline"
                        onClick={(e) => {
                          e.stopPropagation()
                          navigate(`/settings?tab=logs&filter=${encodeURIComponent(error.instance_id)}`)
                        }}
                      >
                        {error.instance_id.substring(0, 8)}
                      </button>
                    ) : (
                      <span className="text-muted-foreground">{'\u2014'}</span>
                    )}
                  </TableCell>
                  <TableCell
                    className="text-muted-foreground"
                    title={error.instance_id}
                  >
                    {error.instance_id.substring(0, 8)}
                  </TableCell>
                  <TableCell className="max-w-md truncate">
                    {error.message}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDateTime(error.created_at)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>

          {totalPages > 1 && (
            <div className="flex items-center justify-between px-4 py-3 text-xs text-muted-foreground border-t">
              <span>
                {startItem}–{endItem} of {total}
              </span>
              <div className="flex gap-1">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page <= 1}
                  onClick={() => setPage((p) => p - 1)}
                  className="h-7 w-7 p-0"
                >
                  <ChevronLeft className="h-4 w-4" />
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page >= totalPages}
                  onClick={() => setPage((p) => p + 1)}
                  className="h-7 w-7 p-0"
                >
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
