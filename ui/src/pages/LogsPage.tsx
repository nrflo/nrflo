import { useState } from 'react'
import { cn } from '@/lib/utils'
import { LogsFinishedTab } from './LogsFinishedTab'
import { LogsLiveTab } from './LogsLiveTab'

type TabId = 'finished' | 'live'

const tabs: { id: TabId; label: string }[] = [
  { id: 'finished', label: 'Finished sessions' },
  { id: 'live', label: 'Live processes' },
]

export function LogsPage() {
  const [tab, setTab] = useState<TabId>('finished')

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

      {tab === 'finished' ? <LogsFinishedTab /> : <LogsLiveTab />}
    </div>
  )
}
