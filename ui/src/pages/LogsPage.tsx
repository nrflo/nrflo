import { useSearchParams } from 'react-router-dom'
import { cn } from '@/lib/utils'
import { LogsFinishedTab } from './LogsFinishedTab'
import { LogsLiveTab } from './LogsLiveTab'
import { LogsSessionsTab } from './LogsSessionsTab'

type TabId = 'sessions' | 'finished' | 'live'

const tabs: { id: TabId; label: string }[] = [
  { id: 'sessions', label: 'Sessions' },
  { id: 'finished', label: 'Finished sessions' },
  { id: 'live', label: 'Live processes' },
]

const tabIds = new Set<string>(tabs.map((t) => t.id))

function isValidTab(value: string | null): value is TabId {
  return value !== null && tabIds.has(value)
}

export function LogsPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const rawTab = searchParams.get('tab')
  const tab: TabId = isValidTab(rawTab) ? rawTab : 'sessions'

  const setTab = (id: TabId) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      next.set('tab', id)
      if (id !== 'sessions') next.delete('sid')
      return next
    })
  }

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-bold">Agent sessions</h1>

      <div className="border-b border-border">
        <div className="flex gap-1">
          {tabs.map(({ id, label }) => (
            <button
              key={id}
              onClick={() => setTab(id)}
              className={cn(
                'flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors',
                tab === id
                  ? 'border-primary text-primary'
                  : 'border-transparent text-muted-foreground hover:text-foreground'
              )}
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      {tab === 'sessions' ? (
        <LogsSessionsTab />
      ) : tab === 'finished' ? (
        <LogsFinishedTab />
      ) : (
        <LogsLiveTab />
      )}
    </div>
  )
}
