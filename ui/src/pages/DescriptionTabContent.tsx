import { Badge } from '@/components/ui/Badge'
import { Card, CardContent } from '@/components/ui/Card'
import { RenderedMarkdown } from '@/components/ui/RenderedMarkdown'
import type { TicketWithDeps } from '@/types/ticket'
import { cn, statusColor, formatDateTime, priorityLabel } from '@/lib/utils'

interface DescriptionTabContentProps {
  ticket: TicketWithDeps
}

export function DescriptionTabContent({ ticket }: DescriptionTabContentProps) {
  return (
    <div className="space-y-6">
      <h2 className="text-xl font-semibold">{ticket.title}</h2>

      <Card>
        <CardContent className="pt-6">
          <dl className="grid grid-cols-2 gap-4">
            <div>
              <dt className="text-sm text-muted-foreground">Priority</dt>
              <dd className="font-medium">{priorityLabel(ticket.priority)}</dd>
            </div>
            <div>
              <dt className="text-sm text-muted-foreground">Type</dt>
              <dd className="font-medium capitalize">{ticket.issue_type}</dd>
            </div>
            <div>
              <dt className="text-sm text-muted-foreground">Created by</dt>
              <dd className="font-medium">{ticket.created_by}</dd>
            </div>
            <div>
              <dt className="text-sm text-muted-foreground">Status</dt>
              <dd>
                <Badge className={cn(statusColor(ticket.status))}>
                  {ticket.status.replace('_', ' ')}
                </Badge>
              </dd>
            </div>
            <div>
              <dt className="text-sm text-muted-foreground">Created</dt>
              <dd className="font-medium">{formatDateTime(ticket.created_at)}</dd>
            </div>
            <div>
              <dt className="text-sm text-muted-foreground">Updated</dt>
              <dd className="font-medium">{formatDateTime(ticket.updated_at)}</dd>
            </div>
            {ticket.closed_at && (
              <div>
                <dt className="text-sm text-muted-foreground">Closed</dt>
                <dd className="font-medium">{formatDateTime(ticket.closed_at)}</dd>
              </div>
            )}
            {ticket.close_reason && (
              <div className="col-span-2">
                <dt className="text-sm text-muted-foreground">Close reason</dt>
                <dd className="font-medium">{ticket.close_reason}</dd>
              </div>
            )}
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="pt-6">
          {ticket.description ? (
            <RenderedMarkdown content={ticket.description} />
          ) : (
            <p className="text-muted-foreground italic">No description</p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
