import { cn } from '@/lib/utils'
import type { ConsoleSkill } from '@/types/consoleChat'

interface ChatComposerSuggestionsProps {
  skills: ConsoleSkill[]
  query: string
  activeIndex: number
  onSelect: (name: string) => void
  onHover: (index: number) => void
}

// Filters skills whose name matches `query` (case-insensitive prefix, falling
// back to substring) — exported so ChatComposer can share the same match set
// when deciding whether the dropdown should be open.
export function filterSkills(skills: ConsoleSkill[], query: string): ConsoleSkill[] {
  const q = query.toLowerCase()
  if (!q) return skills
  const prefix = skills.filter((s) => s.name.toLowerCase().startsWith(q))
  if (prefix.length > 0) return prefix
  return skills.filter((s) => s.name.toLowerCase().includes(q))
}

// Presentational '/' skill-suggestion dropdown, anchored above ChatComposer's
// textarea. Keyboard nav (Arrow/Enter/Escape) is owned by ChatComposer; this
// component only renders the filtered list and reports hover/click.
export function ChatComposerSuggestions({
  skills,
  query,
  activeIndex,
  onSelect,
  onHover,
}: ChatComposerSuggestionsProps) {
  const matches = filterSkills(skills, query)
  if (matches.length === 0) return null

  return (
    <div className="mb-1 max-h-60 w-full overflow-y-auto rounded-md border border-border bg-background shadow-lg">
      {matches.map((skill, index) => (
        <div
          key={skill.name}
          onMouseDown={(e) => {
            e.preventDefault()
            onSelect(skill.name)
          }}
          onMouseEnter={() => onHover(index)}
          className={cn(
            'flex items-center gap-2 min-w-0 px-3 py-2 text-sm cursor-pointer transition-colors',
            index === activeIndex ? 'bg-muted' : 'hover:bg-muted'
          )}
        >
          <span className="shrink-0 font-mono text-xs text-foreground">/{skill.name}</span>
          {skill.description && (
            <span className="truncate text-xs text-muted-foreground">{skill.description}</span>
          )}
        </div>
      ))}
    </div>
  )
}
