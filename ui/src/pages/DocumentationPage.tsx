import { RefreshCw } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { RenderedMarkdown } from '@/components/ui/RenderedMarkdown'
import { Spinner } from '@/components/ui/Spinner'
import { useAgentManual } from '@/hooks/useDocs'

export function DocumentationPage() {
  const { data, isLoading, error, refetch, isFetching } = useAgentManual()

  return (
    <div className="max-w-7xl mx-auto p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Agent Documentation</h1>
        <Button
          variant="outline"
          size="sm"
          onClick={() => refetch()}
          disabled={isFetching}
        >
          <RefreshCw className={`h-4 w-4 mr-2 ${isFetching ? 'animate-spin' : ''}`} />
          Refresh
        </Button>
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
