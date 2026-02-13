import { useState, useMemo } from 'react'
import { Input } from '@/components/ui/Input'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import { useTicketList } from '@/hooks/useTickets'
import { cn, statusColor } from '@/lib/utils'
import type { PendingTicket } from '@/types/ticket'

interface ChainTicketSelectorProps {
  selectedIds: string[]
  onChange: (ids: string[]) => void
}

export function ChainTicketSelector({ selectedIds, onChange }: ChainTicketSelectorProps) {
  const [search, setSearch] = useState('')
  const { data, isLoading } = useTicketList({ status: 'open' })

  const tickets = data?.tickets ?? []

  const filtered = useMemo(() => {
    if (!search) return tickets
    const q = search.toLowerCase()
    return tickets.filter(
      (t: PendingTicket) =>
        t.id.toLowerCase().includes(q) || t.title.toLowerCase().includes(q)
    )
  }, [tickets, search])

  const toggle = (id: string) => {
    if (selectedIds.includes(id)) {
      onChange(selectedIds.filter((s) => s !== id))
    } else {
      onChange([...selectedIds, id])
    }
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Spinner />
      </div>
    )
  }

  return (
    <div className="space-y-2">
      <Input
        placeholder="Search tickets..."
        value={search}
        onChange={(e) => setSearch(e.target.value)}
      />
      {selectedIds.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {selectedIds.length} ticket{selectedIds.length !== 1 ? 's' : ''} selected
        </p>
      )}
      <div className="max-h-60 overflow-y-auto border border-border rounded-lg divide-y divide-border">
        {filtered.length === 0 ? (
          <p className="p-3 text-sm text-muted-foreground text-center">
            No open tickets found
          </p>
        ) : (
          filtered.map((ticket: PendingTicket) => {
            const selected = selectedIds.includes(ticket.id)
            return (
              <label
                key={ticket.id}
                className={cn(
                  'flex items-center gap-3 px-3 py-2 cursor-pointer hover:bg-muted/50 transition-colors',
                  selected && 'bg-muted'
                )}
              >
                <input
                  type="checkbox"
                  checked={selected}
                  onChange={() => toggle(ticket.id)}
                  className="rounded border-border"
                />
                <span className="text-xs font-mono text-muted-foreground shrink-0">
                  {ticket.id}
                </span>
                <span className="text-sm truncate flex-1">{ticket.title}</span>
                <Badge className={statusColor(ticket.status)}>
                  {ticket.status}
                </Badge>
              </label>
            )
          })
        )}
      </div>
    </div>
  )
}
