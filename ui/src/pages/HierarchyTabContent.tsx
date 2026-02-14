import { Link } from 'react-router-dom'
import { ExternalLink, Layers, X } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { Badge } from '@/components/ui/Badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card'
import { TicketSearchDropdown } from '@/components/ui/TicketSearchDropdown'
import { addDependency, removeDependency } from '@/api/tickets'
import { ticketKeys } from '@/hooks/useTickets'
import { cn, statusColor, priorityLabel } from '@/lib/utils'
import type { TicketWithDeps } from '@/types/ticket'
import type { PendingTicket } from '@/types/ticket'

interface HierarchyTabContentProps {
  ticket: TicketWithDeps
}

export function HierarchyTabContent({ ticket }: HierarchyTabContentProps) {
  const queryClient = useQueryClient()

  const handleAddBlocker = async (selected: PendingTicket) => {
    try {
      await addDependency({ issue_id: ticket.id, depends_on_id: selected.id })
      queryClient.invalidateQueries({ queryKey: ticketKeys.detail(ticket.id) })
      queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })
    } catch {
      // ignore
    }
  }

  const handleRemoveBlocker = async (dependsOnId: string) => {
    await removeDependency({ issue_id: ticket.id, depends_on_id: dependsOnId })
    queryClient.invalidateQueries({ queryKey: ticketKeys.detail(ticket.id) })
    queryClient.invalidateQueries({ queryKey: ticketKeys.lists() })
  }

  const hasBlockers = (ticket.blockers?.length ?? 0) > 0
  const hasBlocks = (ticket.blocks?.length ?? 0) > 0
  const hasParent = !!ticket.parent_ticket_id
  const isEpic = ticket.issue_type === 'epic'
  const hasChildren = isEpic && ticket.children && ticket.children.length > 0
  const hasSiblings = hasParent && ticket.siblings && ticket.siblings.length > 0

  return (
    <div className="space-y-6">
      {/* Blockers section */}
      <Card>
        <CardHeader>
          <CardTitle>Blocked by</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {hasBlockers && (
            <div className="space-y-1">
              {ticket.blockers?.map((dep) => (
                <div key={dep.depends_on_id} className="flex items-center gap-2">
                  <Link
                    to={`/tickets/${encodeURIComponent(dep.depends_on_id)}`}
                    className="flex items-center gap-2 text-sm text-primary hover:underline"
                  >
                    <ExternalLink className="h-3 w-3" />
                    <span className="font-mono text-xs">{dep.depends_on_id}</span>
                    {dep.depends_on_title && (
                      <span>{dep.depends_on_title}</span>
                    )}
                  </Link>
                  <button
                    onClick={() => handleRemoveBlocker(dep.depends_on_id)}
                    className="text-muted-foreground hover:text-destructive"
                    title="Remove blocker"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </div>
              ))}
            </div>
          )}
          <TicketSearchDropdown
            onSelect={handleAddBlocker}
            excludeIds={[ticket.id, ...(ticket.blockers?.map((b) => b.depends_on_id) ?? [])]}
            placeholder="Search tickets to add blocker..."
          />
        </CardContent>
      </Card>

      {/* Blocks section */}
      {hasBlocks && (
        <Card>
          <CardHeader>
            <CardTitle>Blocks</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-1">
              {ticket.blocks?.map((dep) => (
                <Link
                  key={dep.issue_id}
                  to={`/tickets/${encodeURIComponent(dep.issue_id)}`}
                  className="flex items-center gap-2 text-sm text-primary hover:underline"
                >
                  <ExternalLink className="h-3 w-3" />
                  <span className="font-mono text-xs">{dep.issue_id}</span>
                  {dep.issue_title && (
                    <span>{dep.issue_title}</span>
                  )}
                </Link>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Epic hierarchy: parent + siblings */}
      {hasParent && (
        <Card>
          <CardHeader>
            <CardTitle>Epic Hierarchy</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <h4 className="text-sm font-medium mb-2">Parent Epic</h4>
              <Link
                to={`/tickets/${encodeURIComponent(ticket.parent_ticket_id!)}`}
                className="flex items-center gap-2 text-sm text-primary hover:underline"
              >
                <Layers className="h-3 w-3" />
                <span className="font-mono text-xs">{ticket.parent_ticket_id}</span>
                {ticket.parent_ticket?.title && (
                  <span>{ticket.parent_ticket.title}</span>
                )}
              </Link>
            </div>
            {hasSiblings && (
              <div>
                <h4 className="text-sm font-medium mb-2">Sibling Tickets</h4>
                <div className="space-y-2">
                  {ticket.siblings!.map((sibling) => (
                    <div
                      key={sibling.id}
                      className={cn(
                        'flex items-center gap-2',
                        sibling.id.toLowerCase() === ticket.id.toLowerCase() && 'bg-muted rounded px-2 py-1'
                      )}
                    >
                      <Badge className={cn(statusColor(sibling.status))}>
                        {sibling.status.replace('_', ' ')}
                      </Badge>
                      <Link
                        to={`/tickets/${encodeURIComponent(sibling.id)}`}
                        className="text-primary hover:underline"
                      >
                        {sibling.id}
                      </Link>
                      <span className="text-sm">{sibling.title}</span>
                      <span className="text-sm text-muted-foreground ml-auto">
                        {priorityLabel(sibling.priority)}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {/* Epic children (when ticket is an epic without a parent showing siblings) */}
      {hasChildren && (
        <Card>
          <CardHeader>
            <CardTitle>Children</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              {ticket.children!.map((child) => (
                <div key={child.id} className="flex items-center gap-2">
                  <Badge className={cn(statusColor(child.status))}>
                    {child.status.replace('_', ' ')}
                  </Badge>
                  <Link
                    to={`/tickets/${encodeURIComponent(child.id)}`}
                    className="text-primary hover:underline"
                  >
                    {child.id}
                  </Link>
                  <span className="text-sm">{child.title}</span>
                  <span className="text-sm text-muted-foreground ml-auto">
                    {priorityLabel(child.priority)}
                  </span>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
