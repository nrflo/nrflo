import { useSearchParams } from 'react-router-dom'
import { RefreshCw } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/Button'
import { RenderedMarkdown } from '@/components/ui/RenderedMarkdown'
import { Spinner } from '@/components/ui/Spinner'
import { useAgentManual } from '@/hooks/useDocs'
import type { DocKind } from '@/types/docs'

const SUB_TABS: { id: DocKind; label: string }[] = [
  { id: 'common', label: 'Common' },
  { id: 'cli', label: 'CLI' },
  { id: 'python', label: 'Python' },
  { id: 'api', label: 'API' },
  { id: 'local-providers', label: 'Local Providers' },
]

const SUB_TAB_IDS = new Set<string>(SUB_TABS.map((t) => t.id))

export function DocumentationPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const subParam = searchParams.get('sub')
  const activeSub: DocKind = subParam && SUB_TAB_IDS.has(subParam) ? (subParam as DocKind) : 'common'

  const { data, isLoading, error, refetch, isFetching } = useAgentManual(activeSub)

  return (
    <div className="w-[85%] mx-auto p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Agent Documentation</h1>
        <Button
          variant="outline"
          size="sm"
          onClick={() => refetch()}
          disabled={isFetching}
        >
          <RefreshCw className={`h-4 w-4 mr-2 ${isFetching ? 'spin-sync' : ''}`} />
          Refresh
        </Button>
      </div>

      <div className="border-b border-border">
        <div className="flex gap-1">
          {SUB_TABS.map(({ id, label }) => (
            <button
              key={id}
              onClick={() => setSearchParams({ sub: id }, { replace: true })}
              className={cn(
                'px-3 py-1 text-xs font-medium border-b-2 transition-colors',
                activeSub === id
                  ? 'border-primary text-primary'
                  : 'border-transparent text-muted-foreground hover:text-foreground'
              )}
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      {isLoading && (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      )}

      {error && (
        <div className="text-center py-12 space-y-3">
          <p className="text-sm text-destructive">
            Failed to load documentation: {error.message}
          </p>
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            Retry
          </Button>
        </div>
      )}

      {!isLoading && !error && data && (
        <div className="rounded-lg border border-border p-6">
          <RenderedMarkdown content={data.content} />
        </div>
      )}
    </div>
  )
}
