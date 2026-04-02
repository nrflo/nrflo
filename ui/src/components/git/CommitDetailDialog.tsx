import { Copy, Check } from 'lucide-react'
import { useState, useMemo } from 'react'
import { Dialog, DialogHeader, DialogBody } from '@/components/ui/Dialog'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import { useGitCommitDetail } from '@/hooks/useGitCommits'
import { formatRelativeTime, formatDateTime } from '@/lib/utils'
import { DiffViewer } from './DiffViewer'
import type { GitChangedFile } from '@/types/git'

function statusBadgeClass(status: string): string {
  switch (status) {
    case 'added':
      return 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200'
    case 'modified':
      return 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200'
    case 'deleted':
      return 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200'
    case 'renamed':
      return 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200'
    default:
      return 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200'
  }
}

function CopyableHash({ hash }: { hash: string }) {
  const [copied, setCopied] = useState(false)

  const handleCopy = () => {
    navigator.clipboard.writeText(hash)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <button
      onClick={handleCopy}
      className="inline-flex items-center gap-1.5 font-mono text-sm text-muted-foreground hover:text-foreground transition-colors"
    >
      {hash}
      {copied ? (
        <Check className="h-3.5 w-3.5 text-green-500" />
      ) : (
        <Copy className="h-3.5 w-3.5" />
      )}
    </button>
  )
}

function ChangedFileRow({ file }: { file: GitChangedFile }) {
  const scrollToFile = () => {
    const el = document.getElementById(`diff-${file.path}`)
    el?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  return (
    <button
      onClick={scrollToFile}
      className="flex items-center gap-3 w-full px-3 py-1.5 text-sm rounded hover:bg-muted transition-colors text-left"
    >
      <Badge className={statusBadgeClass(file.status)}>{file.status}</Badge>
      <span className="font-mono text-xs truncate flex-1">{file.path}</span>
      <span className="text-xs text-muted-foreground whitespace-nowrap">
        {file.additions > 0 && (
          <span className="text-green-600 dark:text-green-400">+{file.additions}</span>
        )}
        {file.additions > 0 && file.deletions > 0 && ' '}
        {file.deletions > 0 && (
          <span className="text-red-600 dark:text-red-400">-{file.deletions}</span>
        )}
      </span>
    </button>
  )
}

interface CommitDetailDialogProps {
  projectId: string
  hash: string | null
  onClose: () => void
}

export function CommitDetailDialog({ projectId, hash, onClose }: CommitDetailDialogProps) {
  const { data, isLoading, error } = useGitCommitDetail(projectId, hash)
  const commit = data?.commit

  const { totalAdditions, totalDeletions } = useMemo(() => {
    if (!commit?.files?.length) return { totalAdditions: 0, totalDeletions: 0 }
    return commit.files.reduce(
      (acc, f) => ({
        totalAdditions: acc.totalAdditions + f.additions,
        totalDeletions: acc.totalDeletions + f.deletions,
      }),
      { totalAdditions: 0, totalDeletions: 0 }
    )
  }, [commit?.files])

  return (
    <Dialog open={!!hash} onClose={onClose} className="max-w-5xl">
      <DialogHeader onClose={onClose}>
        <h2 className="text-lg font-semibold">Commit Details</h2>
      </DialogHeader>
      <DialogBody className="max-h-[75vh]">
        {isLoading && (
          <div className="flex justify-center py-8">
            <Spinner size="lg" />
          </div>
        )}

        {error && (
          <p className="text-sm text-destructive">
            Failed to load commit details: {error.message}
          </p>
        )}

        {commit && (
          <div className="space-y-6">
            {/* Header info */}
            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <span className="text-sm text-muted-foreground">Commit:</span>
                <CopyableHash hash={commit.hash} />
              </div>
              <div className="text-sm">
                <span className="text-muted-foreground">Author: </span>
                <span>{commit.author}</span>
                <span className="text-muted-foreground"> &lt;{commit.author_email}&gt;</span>
              </div>
              <div className="text-sm">
                <span className="text-muted-foreground">Date: </span>
                <span>{formatRelativeTime(commit.date)}</span>
                <span className="text-muted-foreground ml-2">
                  ({formatDateTime(commit.date)})
                </span>
              </div>
              {(totalAdditions > 0 || totalDeletions > 0) && (
                <div className="text-sm">
                  <span className="text-muted-foreground">Changes: </span>
                  {totalAdditions > 0 && (
                    <span className="text-green-600 dark:text-green-400">+{totalAdditions}</span>
                  )}
                  {totalAdditions > 0 && totalDeletions > 0 && ' '}
                  {totalDeletions > 0 && (
                    <span className="text-red-600 dark:text-red-400">-{totalDeletions}</span>
                  )}
                </div>
              )}
              <div className="mt-3 p-3 bg-muted rounded-lg">
                <p className="text-sm whitespace-pre-wrap">{commit.message}</p>
              </div>
            </div>

            {/* Changed files */}
            {commit.files && commit.files.length > 0 && (
              <div>
                <h3 className="text-sm font-medium mb-2">
                  Changed files ({commit.files.length})
                </h3>
                <div className="border border-border rounded-lg divide-y divide-border">
                  {commit.files.map((file) => (
                    <ChangedFileRow key={file.path} file={file} />
                  ))}
                </div>
              </div>
            )}

            {/* Diff */}
            <div>
              <h3 className="text-sm font-medium mb-2">Diff</h3>
              <DiffViewer diff={commit.diff} />
            </div>
          </div>
        )}
      </DialogBody>
    </Dialog>
  )
}
