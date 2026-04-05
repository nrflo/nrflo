import { useState, useRef, useEffect } from 'react'
import { PlusCircle, CheckCircle2, Cpu, Clock, Check } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useDailyStats } from '@/hooks/useTickets'
import { formatTokenCount, formatDurationSec } from '@/lib/utils'
import { useProjectStore } from '@/stores/projectStore'

type StatsRange = 'today' | 'week' | 'month' | 'all'

const RANGE_OPTIONS: { value: StatsRange; label: string }[] = [
  { value: 'today', label: 'Today' },
  { value: 'week', label: 'This Week' },
  { value: 'month', label: 'This Month' },
  { value: 'all', label: 'All Time' },
]

const RANGE_BADGES: Record<StatsRange, string> = {
  today: '',
  week: '7d',
  month: '30d',
  all: 'all',
}

function getStorageKey(projectId: string) {
  return `nrf_daily_stats_range_${projectId}`
}

function loadRange(projectId: string): StatsRange {
  try {
    const stored = localStorage.getItem(getStorageKey(projectId))
    if (stored === 'week' || stored === 'month' || stored === 'all') return stored
  } catch { /* localStorage unavailable */ }
  return 'today'
}

export function DailyStats() {
  const currentProject = useProjectStore((s) => s.currentProject)
  const [range, setRange] = useState<StatsRange>(() => loadRange(currentProject))
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  const { data, isLoading } = useDailyStats(range)

  useEffect(() => {
    setRange(loadRange(currentProject))
  }, [currentProject])

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

  if (isLoading || !data) return null

  function selectRange(value: StatsRange) {
    setRange(value)
    try { localStorage.setItem(getStorageKey(currentProject), value) } catch { /* noop */ }
    setOpen(false)
  }

  return (
    <div ref={ref} className="relative hidden md:flex">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex items-center gap-3 text-xs text-muted-foreground cursor-pointer hover:bg-accent/50 rounded-md px-2 py-1 transition-colors"
      >
        <div className="flex items-center gap-1">
          <PlusCircle className="h-3.5 w-3.5" />
          <span>{data.tickets_created} created</span>
        </div>
        <div className="flex items-center gap-1">
          <CheckCircle2 className="h-3.5 w-3.5" />
          <span>{data.tickets_closed} closed</span>
        </div>
        <div className="flex items-center gap-1">
          <Cpu className="h-3.5 w-3.5" />
          <span>{formatTokenCount(data.tokens_spent)} tokens</span>
        </div>
        <div className="flex items-center gap-1">
          <Clock className="h-3.5 w-3.5" />
          <span>{formatDurationSec(data.agent_time_sec)}</span>
        </div>
        {range !== 'today' && (
          <span className="text-[10px] text-muted-foreground/70 font-medium">
            ({RANGE_BADGES[range]})
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-1 min-w-[160px] rounded-md border border-border bg-background shadow-lg z-50">
          <div className="py-1">
            {RANGE_OPTIONS.map((opt) => (
              <div
                key={opt.value}
                onClick={() => selectRange(opt.value)}
                className={cn(
                  'flex items-center gap-2 px-3 py-2 text-sm cursor-pointer transition-colors',
                  opt.value === range ? 'bg-muted text-foreground' : 'text-foreground hover:bg-muted'
                )}
              >
                <Check className={cn('h-3.5 w-3.5 shrink-0', opt.value === range ? 'opacity-100' : 'opacity-0')} />
                <span>{opt.label}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
