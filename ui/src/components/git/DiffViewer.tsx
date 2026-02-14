import { useMemo } from 'react'

interface FileDiffSection {
  filename: string
  lines: string[]
}

function parseUnifiedDiff(rawDiff: string): FileDiffSection[] {
  const files: FileDiffSection[] = []
  const fileSections = rawDiff.split(/^diff --git /m).filter(Boolean)

  for (const section of fileSections) {
    const allLines = section.split('\n')
    const headerMatch = allLines[0]?.match(/a\/(.+?) b\/(.+)/)
    const filename = headerMatch?.[2] ?? headerMatch?.[1] ?? 'unknown'

    // Collect only hunk lines (starting from @@ markers)
    const diffLines: string[] = []
    let inHunk = false
    for (const line of allLines) {
      if (line.startsWith('@@')) {
        inHunk = true
        diffLines.push(line)
        continue
      }
      if (!inHunk) continue
      // Stop at next file diff boundary
      if (line.startsWith('diff --git ')) break
      diffLines.push(line)
    }

    if (diffLines.length > 0) {
      files.push({ filename, lines: diffLines })
    }
  }

  return files
}

function DiffLine({ line }: { line: string }) {
  let bgClass = ''
  if (line.startsWith('+')) {
    bgClass = 'bg-green-50 dark:bg-green-950/40 text-green-900 dark:text-green-200'
  } else if (line.startsWith('-')) {
    bgClass = 'bg-red-50 dark:bg-red-950/40 text-red-900 dark:text-red-200'
  } else if (line.startsWith('@@')) {
    bgClass = 'bg-blue-50 dark:bg-blue-950/30 text-blue-700 dark:text-blue-300'
  }

  return (
    <div className={`${bgClass} px-4 font-mono text-xs leading-6 whitespace-pre`}>
      {line}
    </div>
  )
}

function FileDiffBlock({ section }: { section: FileDiffSection }) {
  return (
    <div id={`diff-${section.filename}`} className="border border-border rounded-lg overflow-hidden">
      <div className="px-4 py-2 bg-muted text-sm font-mono text-muted-foreground border-b border-border">
        {section.filename}
      </div>
      <div className="overflow-x-auto max-h-[600px] overflow-y-auto">
        {section.lines.map((line, i) => (
          <DiffLine key={i} line={line} />
        ))}
      </div>
    </div>
  )
}

interface DiffViewerProps {
  diff: string
}

export function DiffViewer({ diff }: DiffViewerProps) {
  const fileSections = useMemo(() => parseUnifiedDiff(diff), [diff])

  if (!diff || fileSections.length === 0) {
    return (
      <p className="text-sm text-muted-foreground italic">No diff available</p>
    )
  }

  return (
    <div className="space-y-4">
      {fileSections.map((section) => (
        <FileDiffBlock key={section.filename} section={section} />
      ))}
    </div>
  )
}
