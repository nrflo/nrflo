import { useState, useEffect } from 'react'
import { Terminal, Monitor } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Spinner } from '@/components/ui/Spinner'
import { cn } from '@/lib/utils'
import { useLogs } from '@/hooks/useLogs'

type LogType = 'be' | 'fe'

const logTabs: { key: LogType; label: string; icon: React.ReactNode }[] = [
  { key: 'be', label: 'BE', icon: <Terminal className="h-4 w-4" /> },
  { key: 'fe', label: 'FE', icon: <Monitor className="h-4 w-4" /> },
]

interface LogsSectionProps {
  initialFilter?: string
}

export function LogsSection({ initialFilter }: LogsSectionProps) {
  const [activeTab, setActiveTab] = useState<LogType>('be')
  const [filterInput, setFilterInput] = useState(initialFilter ?? '')
  const [debouncedFilter, setDebouncedFilter] = useState(initialFilter ?? '')

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedFilter(filterInput), 300)
    return () => clearTimeout(timer)
  }, [filterInput])

  useEffect(() => {
    if (initialFilter && activeTab !== 'be') setActiveTab('be')
  }, [initialFilter]) // eslint-disable-line react-hooks/exhaustive-deps

  const { data, isLoading, error, refetch } = useLogs(activeTab, debouncedFilter || undefined)

  return (
    <div className="space-y-6">
      <div className="border-b border-border">
        <div className="flex gap-1">
          {logTabs.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={cn(
                'flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors',
                activeTab === tab.key
                  ? 'border-primary text-primary'
                  : 'border-transparent text-muted-foreground hover:text-foreground'
              )}
            >
              {tab.icon}
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      <div className="flex items-center gap-3">
        <Input
          placeholder="Filter logs..."
          value={filterInput}
          onChange={(e) => setFilterInput(e.target.value)}
          className="max-w-sm h-8 text-sm"
        />
        <span className="text-xs text-muted-foreground whitespace-nowrap">
          {debouncedFilter ? 'Showing all matching lines' : 'Showing last 1000 lines'}
        </span>
      </div>

      {isLoading && (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      )}

      {error && (
        <div className="text-center py-12 space-y-3">
          <p className="text-sm text-destructive">
            Failed to load logs: {error.message}
          </p>
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            Retry
          </Button>
        </div>
      )}

      {!isLoading && !error && (
        <div className="rounded-lg border border-border overflow-auto max-h-[calc(100vh-16rem)]">
          {data?.lines.length === 0 ? (
            <div className="px-4 py-8 text-center text-sm text-muted-foreground">
              No log lines available.
            </div>
          ) : (
            data?.lines.map((line, i) => (
              <div key={i} className="px-4 py-1 font-mono text-sm whitespace-pre border-b border-border last:border-b-0">
                {line}
              </div>
            ))
          )}
        </div>
      )}
    </div>
  )
}
