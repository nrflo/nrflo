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
  { key: 'id', label: 'ID', className: 'w-32' },
  { key: 'title', label: 'Title' },
  { key: 'issue_type', label: 'Type', className: 'w-16' },
  { key: 'status', label: 'Status', className: 'w-24' },
  { key: 'priority', label: 'Priority', className: 'w-20' },
  { key: 'created_by', label: 'Created By', className: 'w-24' },
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
      <table className="w-full text-xs font-mono border-collapse">
        <thead>
          <tr className="text-left text-muted-foreground border-b border-border">
            {COLUMNS.map((col) => (
              <th
                key={col.key}
                className={cn('py-1.5 pr-3 font-medium cursor-pointer select-none hover:text-foreground', col.className)}
                onClick={() => onSortChange(col.key)}
              >
                {col.label}
                <SortIndicator column={col.key} sortBy={sortBy} sortOrder={sortOrder} />
              </th>
            ))}
            <th className="py-1.5 font-medium w-16">Progress</th>
          </tr>
        </thead>
        <tbody>
          {tickets.map((ticket) => {
            const isBlocked = isPendingTicket(ticket) && ticket.is_blocked
            const progress = isPendingTicket(ticket) ? ticket.workflow_progress : undefined

            return (
              <tr
                key={ticket.id}
                onClick={() => navigate(`/tickets/${encodeURIComponent(ticket.id)}`)}
                className="border-b border-border/50 hover:bg-muted/50 cursor-pointer transition-colors"
              >
                <td className="py-1.5 pr-3">
                  <span className="flex items-center gap-1">
                    {ticket.id}
                    {isBlocked && (
                      <span title="Blocked">
                        <Lock className="h-3 w-3 text-orange-500" />
                      </span>
                    )}
                  </span>
                </td>
                <td className="py-1.5 pr-3 max-w-xs truncate">{ticket.title}</td>
                <td className="py-1.5 pr-3">
                  <IssueTypeIcon type={ticket.issue_type} />
                </td>
                <td className="py-1.5 pr-3">
                  <Badge className={cn('text-[10px] px-1 py-0', statusColor(ticket.status))}>
                    {ticket.status.replace('_', ' ')}
                  </Badge>
                </td>
                <td className="py-1.5 pr-3">{priorityLabel(ticket.priority)}</td>
                <td className="py-1.5 pr-3 text-muted-foreground truncate max-w-[6rem]">
                  {ticket.created_by || '-'}
                </td>
                <td className="py-1.5 pr-3 text-muted-foreground">
                  {formatRelativeTime(ticket.updated_at)}
                </td>
                <td className="py-1.5">
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
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
