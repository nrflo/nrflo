import { Link } from 'react-router-dom'
import { ExternalLink } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card'
import type { TicketWithDeps } from '@/types/ticket'

interface DescriptionTabContentProps {
  ticket: TicketWithDeps
}

export function DescriptionTabContent({ ticket }: DescriptionTabContentProps) {
  return (
    <div className="space-y-6">
      <Card>
        <CardContent className="pt-6">
          {ticket.description ? (
            <p className="whitespace-pre-wrap">{ticket.description}</p>
          ) : (
            <p className="text-muted-foreground italic">No description</p>
          )}
        </CardContent>
      </Card>

      {((ticket.blockers?.length ?? 0) > 0 || (ticket.blocks?.length ?? 0) > 0) && (
        <Card>
          <CardHeader>
            <CardTitle>Dependencies</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {(ticket.blockers?.length ?? 0) > 0 && (
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
                      {dep.depends_on_id}
                    </Link>
                  ))}
                </div>
              </div>
            )}
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
      )}
    </div>
  )
}
