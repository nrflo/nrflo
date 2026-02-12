import { Link } from 'react-router-dom'
import { ExternalLink, Layers, X } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card'
import { TicketSearchDropdown } from '@/components/ui/TicketSearchDropdown'
import { addDependency, removeDependency } from '@/api/tickets'
import { ticketKeys } from '@/hooks/useTickets'
import type { TicketWithDeps } from '@/types/ticket'
import type { PendingTicket } from '@/types/ticket'

interface DescriptionTabContentProps {
  ticket: TicketWithDeps
}

export function DescriptionTabContent({ ticket }: DescriptionTabContentProps) {
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

  return (
    <div className="space-y-6">
      {ticket.parent_ticket_id && (
        <Card>
          <CardContent className="pt-6">
            <h4 className="text-sm font-medium mb-2">Parent Epic</h4>
            <Link
              to={`/tickets/${encodeURIComponent(ticket.parent_ticket_id)}`}
              className="flex items-center gap-2 text-sm text-primary hover:underline"
            >
              <Layers className="h-3 w-3" />
              {ticket.parent_ticket_id}
            </Link>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent className="pt-6">
          {ticket.description ? (
            <p className="whitespace-pre-wrap">{ticket.description}</p>
          ) : (
            <p className="text-muted-foreground italic">No description</p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Dependencies</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div>
            <h4 className="text-sm font-medium mb-2">Blocked by</h4>
            {(ticket.blockers?.length ?? 0) > 0 && (
              <div className="space-y-1 mb-2">
                {ticket.blockers?.map((dep) => (
                  <div key={dep.depends_on_id} className="flex items-center gap-2">
                    <Link
                      to={`/tickets/${encodeURIComponent(dep.depends_on_id)}`}
                      className="flex items-center gap-2 text-sm text-primary hover:underline"
                    >
                      <ExternalLink className="h-3 w-3" />
                      {dep.depends_on_id}
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
              placeholder="Search tickets to add..."
            />
          </div>

          {(ticket.blocks?.length ?? 0) > 0 && (
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
                    {dep.issue_id}
                  </Link>
                ))}
              </div>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
