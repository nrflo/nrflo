import { Link } from 'react-router-dom'
import { Lock } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/Card'
import { Badge } from '@/components/ui/Badge'
import { IssueTypeIcon } from '@/components/tickets/IssueTypeIcon'
import { cn, statusColor, formatRelativeTime, priorityLabel } from '@/lib/utils'
import type { Ticket, PendingTicket } from '@/types/ticket'

interface TicketCardProps {
  ticket: Ticket | PendingTicket
}

function isPendingTicket(ticket: Ticket | PendingTicket): ticket is PendingTicket {
  return 'is_blocked' in ticket
}

export function TicketCard({ ticket }: TicketCardProps) {
  const isBlocked = isPendingTicket(ticket) && ticket.is_blocked

  return (
    <Link to={`/tickets/${encodeURIComponent(ticket.id)}`}>
      <Card className="hover:border-primary/50 transition-colors cursor-pointer">
        <CardContent className="p-4">
          <div className="flex items-start gap-3">
            <IssueTypeIcon type={ticket.issue_type} />
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2 mb-1">
                <span className="text-xs text-muted-foreground font-mono">
                  {ticket.id}
                </span>
                {isBlocked && (
                  <span title="Blocked">
                    <Lock className="h-3 w-3 text-orange-500" />
                  </span>
                )}
              </div>
              <h3 className="font-medium text-sm truncate">{ticket.title}</h3>
              {ticket.description && (
                <p className="text-xs text-muted-foreground mt-1 line-clamp-2">
                  {ticket.description}
                </p>
              )}
              <div className="flex items-center gap-2 mt-2">
                <Badge className={cn('text-xs', statusColor(ticket.status))}>
                  {ticket.status.replace('_', ' ')}
                </Badge>
                <span className="text-xs text-muted-foreground">
                  {priorityLabel(ticket.priority)}
                </span>
                <span className="text-xs text-muted-foreground ml-auto">
                  {formatRelativeTime(ticket.updated_at)}
                </span>
              </div>
              {isPendingTicket(ticket) && ticket.workflow_progress && (
                <div className="flex items-center gap-2 mt-2">
                  <div className="flex-1 h-1.5 bg-muted rounded-full overflow-hidden">
                    <div
                      className="h-full bg-primary rounded-full transition-all"
                      style={{
                        width: `${ticket.workflow_progress.total_phases > 0 ? Math.round((ticket.workflow_progress.completed_phases / ticket.workflow_progress.total_phases) * 100) : 0}%`,
                      }}
                    />
                  </div>
                  <span className="text-xs text-muted-foreground whitespace-nowrap">
                    {ticket.workflow_progress.completed_phases}/{ticket.workflow_progress.total_phases} phases
                  </span>
                </div>
              )}
            </div>
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}
