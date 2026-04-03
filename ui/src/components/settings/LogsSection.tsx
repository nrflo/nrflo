import { useState, useEffect } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Spinner } from '@/components/ui/Spinner'
import { useLogs } from '@/hooks/useLogs'

interface LogsSectionProps {
  initialFilter?: string
}

export function LogsSection({ initialFilter }: LogsSectionProps) {
  const [filterInput, setFilterInput] = useState(initialFilter ?? '')
  const [debouncedFilter, setDebouncedFilter] = useState(initialFilter ?? '')

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedFilter(filterInput), 300)
    return () => clearTimeout(timer)
  }, [filterInput])

  const { data, isLoading, error, refetch } = useLogs('be', debouncedFilter || undefined)

  return (
    <div className="space-y-6">
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
