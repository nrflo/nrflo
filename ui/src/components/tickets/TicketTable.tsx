import { useNavigate } from 'react-router-dom'
import { Lock, ChevronUp, ChevronDown } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import { IssueTypeIcon } from '@/components/tickets/IssueTypeIcon'
import { cn, statusColor, formatRelativeTime, priorityLabel } from '@/lib/utils'
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
    <div className="overflow-x-auto">
      <div className="border border-border rounded-lg text-sm font-mono">
        <div className="px-4 py-2 border-b border-border bg-muted/30">
          <div className="flex items-center gap-4 text-xs font-medium text-muted-foreground uppercase tracking-wider" data-testid="table-header">
            {COLUMNS.map((col) => (
              <span
                key={col.key}
                className={cn(
                  'cursor-pointer select-none hover:text-foreground shrink-0',
                  col.key === 'title' ? 'flex-1 min-w-0' : col.className,
                )}
                onClick={() => onSortChange(col.key)}
              >
                {col.label}
                <SortIndicator column={col.key} sortBy={sortBy} sortOrder={sortOrder} />
              </span>
            ))}
            <span className="w-16 shrink-0">Progress</span>
          </div>
        </div>
        {tickets.map((ticket) => {
          const isBlocked = isPendingTicket(ticket) && ticket.is_blocked
          const progress = isPendingTicket(ticket) ? ticket.workflow_progress : undefined

          return (
            <div
              key={ticket.id}
              onClick={() => navigate(`/tickets/${encodeURIComponent(ticket.id)}`)}
              className="flex items-center gap-4 px-4 py-3 border-b border-border last:border-b-0 hover:bg-muted/50 cursor-pointer transition-colors"
              data-testid="table-row"
            >
              <span className="w-10 shrink-0">
                <IssueTypeIcon type={ticket.issue_type} />
              </span>
              <span className="w-32 shrink-0 flex items-center gap-1">
                {ticket.id}
                {isBlocked && (
                  <span title="Blocked">
                    <Lock className="h-3 w-3 text-orange-500" />
                  </span>
                )}
              </span>
              <span className="flex-1 min-w-0 truncate">{ticket.title}</span>
              <span className="w-24 shrink-0">
                <Badge className={cn('text-xs px-1 py-0', statusColor(ticket.status))}>
                  {ticket.status.replace('_', ' ')}
                </Badge>
              </span>
              <span className="w-20 shrink-0">{priorityLabel(ticket.priority)}</span>
              <span className="w-28 shrink-0 text-muted-foreground truncate">
                {ticket.created_by || '-'}
              </span>
              <span className="w-24 shrink-0 text-muted-foreground">
                {formatRelativeTime(ticket.updated_at)}
              </span>
              <span className="w-16 shrink-0">
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
              </span>
            </div>
          )
        })}
      </div>
    </div>
  )
}
