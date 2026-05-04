import { cn } from '@/lib/utils'

interface DiffLine {
  type: 'same' | 'added' | 'removed'
  content: string
}

const MAX_LINES = 500

function computeDiff(a: string, b: string): DiffLine[] {
  const aLines = a.split('\n').slice(0, MAX_LINES)
  const bLines = b.split('\n').slice(0, MAX_LINES)
  const m = aLines.length
  const n = bLines.length

  const dp: number[][] = Array.from({ length: m + 1 }, () => new Array(n + 1).fill(0))
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      dp[i][j] =
        aLines[i - 1] === bLines[j - 1]
          ? dp[i - 1][j - 1] + 1
          : Math.max(dp[i - 1][j], dp[i][j - 1])
    }
  }

  const result: DiffLine[] = []
  let i = m
  let j = n
  while (i > 0 || j > 0) {
    if (i > 0 && j > 0 && aLines[i - 1] === bLines[j - 1]) {
      result.unshift({ type: 'same', content: aLines[i - 1] })
      i--
      j--
    } else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) {
      result.unshift({ type: 'added', content: bLines[j - 1] })
      j--
    } else {
      result.unshift({ type: 'removed', content: aLines[i - 1] })
      i--
    }
  }
  return result
}

interface Props {
  before: string
  after: string
  label?: string
}

export function DiffPreview({ before, after, label }: Props) {
  if (!before && !after) return null

  const diff = computeDiff(before, after)
  const hasChanges = diff.some((l) => l.type !== 'same')

  return (
    <div className="border border-border rounded-lg overflow-hidden">
      {label && (
        <div className="px-4 py-2 bg-muted text-xs font-medium border-b border-border">
          {label}
        </div>
      )}
      {!hasChanges ? (
        <p className="px-4 py-3 text-sm text-muted-foreground">No changes</p>
      ) : (
        <pre className="text-xs overflow-auto max-h-80 p-0 m-0">
          {diff.map((line, idx) => (
            <div
              key={idx}
              className={cn(
                'px-4 py-0.5',
                line.type === 'added' &&
                  'bg-green-500/10 text-green-700 dark:text-green-400',
                line.type === 'removed' &&
                  'bg-red-500/10 text-red-700 dark:text-red-400',
                line.type === 'same' && 'text-muted-foreground'
              )}
            >
              {line.type === 'added' ? '+ ' : line.type === 'removed' ? '- ' : '  '}
              {line.content}
            </div>
          ))}
        </pre>
      )}
    </div>
  )
}
