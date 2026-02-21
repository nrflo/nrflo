import { useCallback } from 'react'
import { ChevronUp, ChevronDown, Lock } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { cn } from '@/lib/utils'

interface ChainOrderItem {
  ticketId: string
  title: string
}

interface ChainOrderListProps {
  items: ChainOrderItem[]
  deps: Record<string, string[]>
  addedByDeps: string[]
  onReorder: (ticketIds: string[]) => void
}

export function ChainOrderList({ items, deps, addedByDeps, onReorder }: ChainOrderListProps) {
  const addedSet = new Set(addedByDeps)

  const canMoveUp = useCallback(
    (index: number) => {
      if (index === 0) return false
      // Moving item[index] up means swapping with item[index-1].
      // Blocked if current ticket depends on the ticket above (above is a blocker).
      const currentId = items[index].ticketId
      const aboveId = items[index - 1].ticketId
      const blockers = deps[currentId] ?? []
      return !blockers.includes(aboveId)
    },
    [items, deps]
  )

  const canMoveDown = useCallback(
    (index: number) => {
      if (index === items.length - 1) return false
      // Moving item[index] down means swapping with item[index+1].
      // Blocked if the ticket below depends on current ticket (current is a blocker).
      const currentId = items[index].ticketId
      const belowId = items[index + 1].ticketId
      const belowBlockers = deps[belowId] ?? []
      return !belowBlockers.includes(currentId)
    },
    [items, deps]
  )

  const moveUp = useCallback(
    (index: number) => {
      if (!canMoveUp(index)) return
      const newOrder = [...items]
      ;[newOrder[index - 1], newOrder[index]] = [newOrder[index], newOrder[index - 1]]
      onReorder(newOrder.map((i) => i.ticketId))
    },
    [items, canMoveUp, onReorder]
  )

  const moveDown = useCallback(
    (index: number) => {
      if (!canMoveDown(index)) return
      const newOrder = [...items]
      ;[newOrder[index], newOrder[index + 1]] = [newOrder[index + 1], newOrder[index]]
      onReorder(newOrder.map((i) => i.ticketId))
    },
    [items, canMoveDown, onReorder]
  )

  if (items.length === 0) return null

  return (
    <div className="space-y-1">
      <p className="text-xs font-medium text-muted-foreground">Execution Order</p>
      <div className="border border-border rounded-lg divide-y divide-border">
        {items.map((item, index) => {
          const hasBlockers = (deps[item.ticketId] ?? []).length > 0
          const isAutoAdded = addedSet.has(item.ticketId)
          return (
            <div
              key={item.ticketId}
              className={cn(
                'flex items-center gap-2 px-3 py-1.5 text-sm',
                isAutoAdded && 'opacity-60'
              )}
            >
              <span className="text-xs text-muted-foreground w-5 text-right shrink-0">
                {index + 1}.
              </span>
              <span className="font-mono text-xs text-muted-foreground shrink-0">
                {item.ticketId}
              </span>
              <span className="truncate flex-1">{item.title}</span>
              {hasBlockers && (
                <Lock className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
              )}
              {isAutoAdded && (
                <Badge variant="secondary" className="shrink-0">auto-added</Badge>
              )}
              <div className="flex items-center gap-0.5 shrink-0">
                <button
                  type="button"
                  onClick={() => moveUp(index)}
                  disabled={!canMoveUp(index)}
                  className="p-0.5 rounded hover:bg-muted disabled:opacity-30 disabled:cursor-not-allowed"
                  aria-label={`Move ${item.ticketId} up`}
                >
                  <ChevronUp className="h-4 w-4" />
                </button>
                <button
                  type="button"
                  onClick={() => moveDown(index)}
                  disabled={!canMoveDown(index)}
                  className="p-0.5 rounded hover:bg-muted disabled:opacity-30 disabled:cursor-not-allowed"
                  aria-label={`Move ${item.ticketId} down`}
                >
                  <ChevronDown className="h-4 w-4" />
                </button>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
