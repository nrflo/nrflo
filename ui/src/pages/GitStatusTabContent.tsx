import { useState } from 'react'
import { RefreshCw, ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { useGitCommits } from '@/hooks/useGitCommits'
import { formatRelativeTime } from '@/lib/utils'
import { CommitDetailDialog } from '@/components/git/CommitDetailDialog'

interface GitStatusTabContentProps {
  projectId: string
}

export function GitStatusTabContent({ projectId }: GitStatusTabContentProps) {
  const [page, setPage] = useState(1)
  const [selectedHash, setSelectedHash] = useState<string | null>(null)
  const perPage = 20

  const { data, isLoading, error, refetch, isFetching } = useGitCommits(
    projectId,
    page,
    perPage
  )

  const commits = data?.commits ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / perPage))

  if (!projectId) {
    return (
      <div className="text-center py-12">
        <p className="text-muted-foreground">No project selected</p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Header with refresh */}
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Git Commits</h2>
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

      {/* Loading state */}
      {isLoading && (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      )}

      {/* Error state */}
      {error && (
        <div className="text-center py-12 space-y-3">
          <p className="text-sm text-destructive">
            {error.message?.includes('400')
              ? 'No git repository configured for this project'
              : `Failed to load commits: ${error.message}`}
          </p>
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            Retry
          </Button>
        </div>
      )}

      {/* Empty state */}
      {!isLoading && !error && commits.length === 0 && (
        <div className="text-center py-12">
          <p className="text-muted-foreground">No commits found</p>
        </div>
      )}

      {/* Commit list */}
      {!isLoading && !error && commits.length > 0 && (
        <>
          <div className="border border-border rounded-lg divide-y divide-border">
            {commits.map((commit) => (
              <button
                key={commit.hash}
                onClick={() => setSelectedHash(commit.hash)}
                className="flex items-center gap-4 w-full px-4 py-3 text-left hover:bg-muted/50 transition-colors"
              >
                <span className="font-mono text-xs text-muted-foreground w-16 shrink-0">
                  {commit.short_hash}
                </span>
                <span className="text-sm truncate flex-1">
                  {commit.message.split('\n')[0]}
                </span>
                <span className="text-xs text-muted-foreground shrink-0">
                  {commit.author}
                </span>
                <span className="text-xs text-muted-foreground shrink-0 w-16 text-right">
                  {formatRelativeTime(commit.date)}
                </span>
              </button>
            ))}
          </div>

          {/* Pagination */}
          <div className="flex items-center justify-between">
            <span className="text-sm text-muted-foreground">
              {total} commit{total !== 1 ? 's' : ''} total
            </span>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={page <= 1}
              >
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <span className="text-sm text-muted-foreground">
                Page {page} of {totalPages}
              </span>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                disabled={page >= totalPages}
              >
                <ChevronRight className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </>
      )}

      {/* Commit detail dialog */}
      <CommitDetailDialog
        projectId={projectId}
        hash={selectedHash}
        onClose={() => setSelectedHash(null)}
      />
    </div>
  )
}
