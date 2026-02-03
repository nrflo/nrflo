import { TicketCard } from './TicketCard'
import { Spinner } from '@/components/ui/Spinner'
import type { Ticket, PendingTicket } from '@/types/ticket'

interface TicketListProps {
  tickets: (Ticket | PendingTicket)[] | undefined
  isLoading: boolean
  error: Error | null
  emptyMessage?: string
}

export function TicketList({
  tickets,
  isLoading,
  error,
  emptyMessage = 'No tickets found',
}: TicketListProps) {
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
    <div className="grid gap-3">
      {tickets.map((ticket) => (
        <TicketCard key={ticket.id} ticket={ticket} />
      ))}
    </div>
  )
}
