import { useState, useMemo, useCallback } from 'react'
import { Input } from '@/components/ui/Input'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import { IssueTypeIcon } from '@/components/tickets/IssueTypeIcon'
import { useTicketList } from '@/hooks/useTickets'
import { cn, statusColor } from '@/lib/utils'
import type { PendingTicket } from '@/types/ticket'

interface ChainTicketSelectorProps {
  selectedIds: string[]
  onChange: (ids: string[]) => void
  onEpicIdsChange?: (epicIds: string[]) => void
  excludeIds?: Set<string>
}

export function ChainTicketSelector({ selectedIds, onChange, onEpicIdsChange, excludeIds }: ChainTicketSelectorProps) {
  const [search, setSearch] = useState('')
  const [activeEpicIds, setActiveEpicIds] = useState<string[]>([])
  const { data, isLoading } = useTicketList({ status: 'open' })

  const tickets = data?.tickets ?? []

  // Map epic ID -> child IDs (direct children only)
  const epicChildMap = useMemo(() => {
    const map = new Map<string, string[]>()
    const epicIds = new Set(tickets.filter((t) => t.issue_type === 'epic').map((t) => t.id))
    for (const t of tickets) {
      if (t.parent_ticket_id && epicIds.has(t.parent_ticket_id)) {
        const children = map.get(t.parent_ticket_id) ?? []
        children.push(t.id)
        map.set(t.parent_ticket_id, children)
      }
    }
    return map
  }, [tickets])

  // Set of child IDs belonging to any active (selected) epic
  const activeEpicChildIds = useMemo(() => {
    const set = new Set<string>()
    for (const epicId of activeEpicIds) {
      for (const childId of epicChildMap.get(epicId) ?? []) {
        set.add(childId)
      }
    }
    return set
  }, [activeEpicIds, epicChildMap])

  const updateEpicIds = useCallback((newEpicIds: string[]) => {
    setActiveEpicIds(newEpicIds)
    onEpicIdsChange?.(newEpicIds)
  }, [onEpicIdsChange])

  const toggle = useCallback((id: string) => {
    const ticket = tickets.find((t) => t.id === id)

    if (ticket?.issue_type === 'epic') {
      const children = epicChildMap.get(id) ?? []
      if (selectedIds.includes(id)) {
        // Deselect epic + all its children
        const toRemove = new Set([id, ...children])
        onChange(selectedIds.filter((s) => !toRemove.has(s)))
        updateEpicIds(activeEpicIds.filter((e) => e !== id))
      } else {
        // Select epic + all its children
        const toAdd = [id, ...children].filter((x) => !selectedIds.includes(x))
        onChange([...selectedIds, ...toAdd])
        updateEpicIds([...activeEpicIds, id])
      }
      return
    }

    // Regular ticket toggle (but not if it's a child of an active epic)
    if (activeEpicChildIds.has(id)) return

    if (selectedIds.includes(id)) {
      onChange(selectedIds.filter((s) => s !== id))
    } else {
      onChange([...selectedIds, id])
    }
  }, [tickets, selectedIds, onChange, epicChildMap, activeEpicIds, activeEpicChildIds, updateEpicIds])

  const filtered = useMemo(() => {
    // Hide children of selected epics and explicitly excluded tickets from the list
    let visible = tickets.filter((t) => !activeEpicChildIds.has(t.id) && !excludeIds?.has(t.id))
    if (search) {
      const q = search.toLowerCase()
      visible = visible.filter(
        (t: PendingTicket) =>
          t.id.toLowerCase().includes(q) || t.title.toLowerCase().includes(q)
      )
    }
    return visible
  }, [tickets, search, activeEpicChildIds, excludeIds])

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
            const isEpic = ticket.issue_type === 'epic'
            const childCount = epicChildMap.get(ticket.id)?.length ?? 0
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
                <IssueTypeIcon type={ticket.issue_type} />
                <span className="text-xs font-mono text-muted-foreground shrink-0">
                  {ticket.id}
                </span>
                <span className="text-sm truncate flex-1">{ticket.title}</span>
                {isEpic && selected && childCount > 0 && (
                  <Badge variant="secondary" className="shrink-0">
                    {childCount} child{childCount !== 1 ? 'ren' : ''} included
                  </Badge>
                )}
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
