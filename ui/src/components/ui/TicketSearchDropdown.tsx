import { useState, useRef, useEffect } from 'react'
import { Search, Loader2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useTicketSearch } from '@/hooks/useTickets'
import type { PendingTicket } from '@/types/ticket'

interface TicketSearchDropdownProps {
  onSelect: (ticket: PendingTicket) => void
  excludeIds?: string[]
  placeholder?: string
}

export function TicketSearchDropdown({
  onSelect,
  excludeIds = [],
  placeholder = 'Search tickets...',
}: TicketSearchDropdownProps) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const ref = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  const { data, isFetching } = useTicketSearch(query)

  const results = (data?.tickets ?? []).filter(
    (t) => !excludeIds.includes(t.id)
  )

  useEffect(() => {
    if (!open) return

    function handleClickOutside(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }

    function handleEscape(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false)
    }

    document.addEventListener('mousedown', handleClickOutside)
    document.addEventListener('keydown', handleEscape)
    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
      document.removeEventListener('keydown', handleEscape)
    }
  }, [open])

  const handleSelect = (ticket: PendingTicket) => {
    onSelect(ticket)
    setQuery('')
    setOpen(false)
  }

  return (
    <div ref={ref} className="relative">
      <div className="relative">
        <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
        <input
          ref={inputRef}
          type="text"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value)
            if (e.target.value.length >= 2) setOpen(true)
          }}
          onFocus={() => {
            if (query.length >= 2) setOpen(true)
          }}
          placeholder={placeholder}
          className={cn(
            'h-8 w-56 rounded-md border border-border bg-background pl-7 pr-8 text-sm',
            'placeholder:text-muted-foreground',
            'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2'
          )}
        />
        {isFetching && (
          <Loader2 className="absolute right-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground animate-spin" />
        )}
      </div>

      {open && query.length >= 2 && (
        <div className="absolute left-0 top-full mt-1 w-80 max-h-60 overflow-y-auto rounded-md border border-border bg-background shadow-lg z-50">
          {results.length === 0 && !isFetching && (
            <div className="px-3 py-2 text-sm text-muted-foreground">
              No tickets found
            </div>
          )}
          {results.slice(0, 10).map((ticket) => (
            <div
              key={ticket.id}
              onClick={() => handleSelect(ticket)}
              className={cn(
                'flex items-center gap-2 px-3 py-2 text-sm cursor-pointer transition-colors',
                'hover:bg-muted'
              )}
            >
              <span className="shrink-0 font-mono text-xs text-muted-foreground">
                {ticket.id}
              </span>
              <span className="truncate text-foreground">{ticket.title}</span>
              <span
                className={cn(
                  'ml-auto shrink-0 rounded-full px-1.5 py-0.5 text-[10px] font-medium',
                  ticket.status === 'open' && 'bg-blue-500/10 text-blue-500',
                  ticket.status === 'in_progress' && 'bg-yellow-500/10 text-yellow-500',
                  ticket.status === 'closed' && 'bg-muted text-muted-foreground'
                )}
              >
                {ticket.status}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
