import { Link } from 'react-router-dom'
import { ExternalLink } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card'
import { RenderedMarkdown } from '@/components/ui/RenderedMarkdown'
import type { TicketWithDeps } from '@/types/ticket'
import { cn, statusColor, formatDateTime, priorityLabel } from '@/lib/utils'

interface DetailsTabContentProps {
  ticket: TicketWithDeps
}

export function DetailsTabContent({ ticket }: DetailsTabContentProps) {
  const hasBlockers = (ticket.blockers?.length ?? 0) > 0
  const hasBlocks = (ticket.blocks?.length ?? 0) > 0

  return (
    <div className="space-y-6">
      {/* Read-only dependency lists */}
      {(hasBlockers || hasBlocks) && (
        <Card>
          <CardHeader>
            <CardTitle>Dependencies</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {hasBlockers && (
              <div>
                <h4 className="text-sm font-medium mb-2">Blocked by</h4>
                <div className="space-y-1">
                  {ticket.blockers?.map((dep) => (
                    <Link
                      key={dep.depends_on_id}
                      to={`/tickets/${encodeURIComponent(dep.depends_on_id)}`}
                      className="flex items-center gap-2 text-sm text-primary hover:underline"
                    >
                      <ExternalLink className="h-3 w-3" />
                      <span className="font-mono text-xs">{dep.depends_on_id}</span>
                      {dep.depends_on_title && <span>{dep.depends_on_title}</span>}
                    </Link>
                  ))}
                </div>
              </div>
            )}
            {hasBlocks && (
              <div>
                <h4 className="text-sm font-medium mb-2">Blocks</h4>
                <div className="space-y-1">
                  {ticket.blocks?.map((dep) => (
                    <Link
                      key={dep.issue_id}
                      to={`/tickets/${encodeURIComponent(dep.issue_id)}`}
                      className="flex items-center gap-2 text-sm text-primary hover:underline"
                    >
                      <ExternalLink className="h-3 w-3" />
                      <span className="font-mono text-xs">{dep.issue_id}</span>
                      {dep.issue_title && <span>{dep.issue_title}</span>}
                    </Link>
                  ))}
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {/* Description text */}
      <Card>
        <CardContent className="pt-6">
          {ticket.description ? (
            <RenderedMarkdown content={ticket.description} />
          ) : (
            <p className="text-muted-foreground italic">No description</p>
          )}
        </CardContent>
      </Card>

      {/* Metadata */}
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
    </div>
  )
}
