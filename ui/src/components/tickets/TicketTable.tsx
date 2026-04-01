import { useNavigate } from 'react-router-dom'
import { Lock, ChevronUp, ChevronDown } from 'lucide-react'
import { Spinner } from '@/components/ui/Spinner'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { StatusCell } from '@/components/ui/StatusCell'
import { IssueTypeIcon } from '@/components/tickets/IssueTypeIcon'
import { cn, formatRelativeTime, priorityLabel } from '@/lib/utils'
import type { Ticket, PendingTicket } from '@/types/ticket'

interface TicketTableProps {
  tickets: (Ticket | PendingTicket)[] | undefined
  isLoading: boolean
  error: Error | null
  emptyMessage?: string
  sortBy: string
  sortOrder: string
  onSortChange: (column: string) => void
}

interface SortableColumn {
  key: string
  label: string
  className?: string
}

const COLUMNS: SortableColumn[] = [
  { key: 'issue_type', label: 'Type', className: 'w-10' },
  { key: 'id', label: 'ID', className: 'w-32' },
  { key: 'title', label: 'Title' },
  { key: 'status', label: 'Status', className: 'w-24' },
  { key: 'priority', label: 'Priority', className: 'w-20' },
  { key: 'created_by', label: 'Created By', className: 'w-28' },
  { key: 'updated_at', label: 'Updated', className: 'w-24' },
]

function isPendingTicket(ticket: Ticket | PendingTicket): ticket is PendingTicket {
  return 'is_blocked' in ticket
}

function SortIndicator({ column, sortBy, sortOrder }: { column: string; sortBy: string; sortOrder: string }) {
  if (column !== sortBy) return null
  return sortOrder === 'asc'
    ? <ChevronUp className="h-3 w-3 inline ml-0.5" />
    : <ChevronDown className="h-3 w-3 inline ml-0.5" />
}

export function TicketTable({
  tickets,
  isLoading,
  error,
  emptyMessage = 'No tickets found',
  sortBy,
  sortOrder,
  onSortChange,
}: TicketTableProps) {
  const navigate = useNavigate()

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner size="lg" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center py-12 text-destructive">
        <p>Error loading tickets: {error.message}</p>
      </div>
    )
  }

  if (!tickets || tickets.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <p>{emptyMessage}</p>
      </div>
    )
  }

  return (
    <Table>
      <TableHeader>
        <TableRow data-testid="table-header">
          {COLUMNS.map((col) => (
            <TableHead
              key={col.key}
              className={cn('cursor-pointer select-none hover:text-foreground', col.className)}
              onClick={() => onSortChange(col.key)}
            >
              {col.label}
              <SortIndicator column={col.key} sortBy={sortBy} sortOrder={sortOrder} />
            </TableHead>
          ))}
          <TableHead className="w-16">Progress</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {tickets.map((ticket) => {
          const isBlocked = isPendingTicket(ticket) && ticket.is_blocked
          const progress = isPendingTicket(ticket) ? ticket.workflow_progress : undefined

          return (
            <TableRow
              key={ticket.id}
              onClick={() => navigate(`/tickets/${encodeURIComponent(ticket.id)}`)}
              className="hover:bg-muted/50 cursor-pointer"
              data-testid="table-row"
            >
              <TableCell className="w-10">
                <IssueTypeIcon type={ticket.issue_type} />
              </TableCell>
              <TableCell className="w-32">
                <span className="flex items-center gap-1">
                  {ticket.id}
                  {isBlocked && (
                    <span title="Blocked">
                      <Lock className="h-3 w-3 text-orange-500" />
                    </span>
                  )}
                </span>
              </TableCell>
              <TableCell className="max-w-0 truncate">{ticket.title}</TableCell>
              <TableCell className="w-24">
                <StatusCell status={ticket.status} />
              </TableCell>
              <TableCell className="w-20">{priorityLabel(ticket.priority)}</TableCell>
              <TableCell className="w-28 text-muted-foreground truncate">
                {ticket.created_by || '-'}
              </TableCell>
              <TableCell className="w-24 text-muted-foreground">
                {formatRelativeTime(ticket.updated_at)}
              </TableCell>
              <TableCell className="w-16">
                {progress && progress.total_phases > 0 ? (
                  <div className="flex items-center gap-1">
                    <div className="flex-1 h-1.5 bg-muted rounded-full overflow-hidden w-12">
                      <div
                        className="h-full bg-primary rounded-full transition-all"
                        style={{
                          width: `${Math.round((progress.completed_phases / progress.total_phases) * 100)}%`,
                        }}
                      />
                    </div>
                    <span className="text-muted-foreground whitespace-nowrap">
                      {progress.completed_phases}/{progress.total_phases}
                    </span>
                  </div>
                ) : null}
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
