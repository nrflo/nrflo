import { Badge } from '@/components/ui/Badge'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import type { NotificationDelivery } from '@/types/notifications'

export function NotificationDeliveriesPanel({ deliveries }: { deliveries: NotificationDelivery[] }) {
  if (!deliveries.length) return null
  return (
    <div className="pt-2 border-t border-border">
      <div className="text-sm font-semibold text-muted-foreground mb-2">Recent Deliveries</div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Time</TableHead>
            <TableHead>Event</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Attempts</TableHead>
            <TableHead>Error</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {deliveries.map((d) => (
            <TableRow key={d.id}>
              <TableCell className="text-xs">{new Date(d.created_at).toLocaleString()}</TableCell>
              <TableCell className="text-xs font-mono">{d.event_type}</TableCell>
              <TableCell>
                <Badge
                  variant={d.status === 'delivered' ? 'success' : d.status === 'failed' ? 'destructive' : 'secondary'}
                  className="text-xs"
                >
                  {d.status}
                </Badge>
              </TableCell>
              <TableCell className="text-xs">{d.attempts}</TableCell>
              <TableCell className="text-xs text-muted-foreground truncate max-w-[200px]">
                {d.last_error || '—'}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
