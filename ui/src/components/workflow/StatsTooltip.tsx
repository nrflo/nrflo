import { HelpCircle } from 'lucide-react'
import { Tooltip } from '@/components/ui/Tooltip'
import { cn } from '@/lib/utils'

interface StatsTooltipProps {
  stats: Record<string, number>
  className?: string
}

function formatStatKey(key: string): string {
  // Truncate long keys (especially skill commands)
  const maxLen = 40
  if (key.length > maxLen) {
    return key.slice(0, maxLen) + '...'
  }
  return key
}

function categorizeStats(stats: Record<string, number>): {
  tools: [string, number][]
  skills: [string, number][]
  other: [string, number][]
} {
  const tools: [string, number][] = []
  const skills: [string, number][] = []
  const other: [string, number][] = []

  for (const [key, count] of Object.entries(stats)) {
    if (key.startsWith('tool:')) {
      tools.push([key, count])
    } else if (key.startsWith('skill:')) {
      skills.push([key, count])
    } else {
      other.push([key, count])
    }
  }

  // Sort by count descending
  tools.sort((a, b) => b[1] - a[1])
  skills.sort((a, b) => b[1] - a[1])
  other.sort((a, b) => b[1] - a[1])

  return { tools, skills, other }
}

function StatsTable({ stats }: { stats: Record<string, number> }) {
  const { tools, skills, other } = categorizeStats(stats)
  const total = Object.values(stats).reduce((sum, count) => sum + count, 0)

  if (total === 0) {
    return <div className="text-muted-foreground text-xs">No stats available</div>
  }

  return (
    <div className="min-w-[200px] max-w-[350px]">
      <div className="text-xs font-medium mb-2">Message Stats</div>

      {/* Tools section */}
      {tools.length > 0 && (
        <div className="mb-2">
          <div className="text-xs text-muted-foreground mb-1">Tools</div>
          <div className="space-y-0.5">
            {tools.map(([key, count]) => (
              <div key={key} className="flex justify-between gap-4 text-xs">
                <span className="font-mono truncate" title={key}>{formatStatKey(key)}</span>
                <span className="text-muted-foreground shrink-0">{count}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Skills section */}
      {skills.length > 0 && (
        <div className="mb-2">
          <div className="text-xs text-muted-foreground mb-1">Skills</div>
          <div className="space-y-0.5">
            {skills.map(([key, count]) => (
              <div key={key} className="flex justify-between gap-4 text-xs">
                <span className="font-mono truncate" title={key}>{formatStatKey(key)}</span>
                <span className="text-muted-foreground shrink-0">{count}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Other section (text, result) */}
      {other.length > 0 && (
        <div className="mb-2">
          <div className="text-xs text-muted-foreground mb-1">Other</div>
          <div className="space-y-0.5">
            {other.map(([key, count]) => (
              <div key={key} className="flex justify-between gap-4 text-xs">
                <span className="font-mono truncate" title={key}>{formatStatKey(key)}</span>
                <span className="text-muted-foreground shrink-0">{count}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Total */}
      <div className="pt-2 border-t border-border flex justify-between text-xs font-medium">
        <span>Total</span>
        <span>{total}</span>
      </div>
    </div>
  )
}

export function StatsTooltip({ stats, className }: StatsTooltipProps) {
  const hasStats = stats && Object.keys(stats).length > 0

  if (!hasStats) {
    return null
  }

  return (
    <Tooltip content={<StatsTable stats={stats} />} position="bottom" delay={200}>
      <span
        role="img"
        aria-label="Stats info"
        className={cn(
          'inline-flex items-center justify-center h-4 w-4 rounded-full',
          'text-muted-foreground hover:text-foreground transition-colors',
          className
        )}
        onClick={(e) => e.stopPropagation()}
      >
        <HelpCircle className="h-3.5 w-3.5" />
      </span>
    </Tooltip>
  )
}
