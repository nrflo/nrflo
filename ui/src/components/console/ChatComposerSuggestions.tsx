import { cn } from '@/lib/utils'

// Shared shape for anything the '/' dropdown can list: project skills,
// the '/invoke' directive row, and the chat's tool catalogue.
export type SuggestionItem = { name: string; description?: string }

interface ChatComposerSuggestionsProps {
  items: SuggestionItem[]
  query: string
  activeIndex: number
  onSelect: (name: string) => void
  onHover: (index: number) => void
}

// Filters items whose name matches `query` (case-insensitive prefix, falling
// back to substring) — exported so ChatComposer can share the same match set
// when deciding whether the dropdown should be open, for both skill and tool
// modes.
export function filterSuggestions<T extends { name: string }>(items: T[], query: string): T[] {
  const q = query.toLowerCase()
  if (!q) return items
  const prefix = items.filter((s) => s.name.toLowerCase().startsWith(q))
  if (prefix.length > 0) return prefix
  return items.filter((s) => s.name.toLowerCase().includes(q))
}

// Presentational '/' suggestion dropdown, anchored above ChatComposer's
// textarea. Keyboard nav (Arrow/Enter/Escape) is owned by ChatComposer; this
// component only renders the filtered list and reports hover/click. Backs
// both the skill dropdown and the '/invoke' tool dropdown.
export function ChatComposerSuggestions({
  items,
  query,
  activeIndex,
  onSelect,
  onHover,
}: ChatComposerSuggestionsProps) {
  const matches = filterSuggestions(items, query)
  if (matches.length === 0) return null

  return (
    <div className="mb-1 max-h-60 w-full overflow-y-auto rounded-md border border-border bg-background shadow-lg">
      {matches.map((item, index) => (
        <div
          key={item.name}
          onMouseDown={(e) => {
            e.preventDefault()
            onSelect(item.name)
          }}
          onMouseEnter={() => onHover(index)}
          className={cn(
            'flex items-center gap-2 min-w-0 px-3 py-2 text-sm cursor-pointer transition-colors',
            index === activeIndex ? 'bg-muted' : 'hover:bg-muted'
          )}
        >
          <span className="shrink-0 font-mono text-xs text-foreground">/{item.name}</span>
          {item.description && (
            <span className="truncate text-xs text-muted-foreground">{item.description}</span>
          )}
        </div>
      ))}
    </div>
  )
}
