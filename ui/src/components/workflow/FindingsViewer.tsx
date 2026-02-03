import { useState } from 'react'
import { Copy, Check } from 'lucide-react'
import { Button } from '@/components/ui/Button'

interface FindingsViewerProps {
  findings: Record<string, unknown>
}

// Try to parse a string as JSON, returning the formatted string or null if not JSON
function tryFormatAsJson(value: string): string | null {
  try {
    const parsed = JSON.parse(value)
    return JSON.stringify(parsed, null, 2)
  } catch {
    return null
  }
}

// Simple finding value renderer - shows strings as-is or JSON-formatted for objects
function SimpleFindingValue({ value }: { value: unknown }): React.ReactNode {
  // If it's a string, try to parse as JSON for pretty formatting
  if (typeof value === 'string') {
    const formatted = tryFormatAsJson(value)
    if (formatted !== null) {
      return (
        <pre className="text-xs font-mono whitespace-pre-wrap break-words">
          {formatted}
        </pre>
      )
    }
    // Not valid JSON, show as-is (no truncation)
    return (
      <span className="text-green-700 dark:text-green-400 whitespace-pre-wrap break-words">
        {value}
      </span>
    )
  }

  // For objects/arrays, stringify to JSON
  if (typeof value === 'object' && value !== null) {
    return (
      <pre className="text-xs font-mono whitespace-pre-wrap break-words">
        {JSON.stringify(value, null, 2)}
      </pre>
    )
  }

  // For primitives (number, boolean, null)
  if (value === null || value === undefined) {
    return <span className="text-muted-foreground italic">null</span>
  }
  return <span>{String(value)}</span>
}

export function FindingsViewer({ findings }: FindingsViewerProps) {
  const [copied, setCopied] = useState(false)

  const copyFindings = async () => {
    await navigator.clipboard.writeText(JSON.stringify(findings, null, 2))
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <h4 className="text-sm font-medium">Findings</h4>
        <Button variant="ghost" size="sm" onClick={copyFindings}>
          {copied ? (
            <Check className="h-3 w-3 mr-1" />
          ) : (
            <Copy className="h-3 w-3 mr-1" />
          )}
          {copied ? 'Copied' : 'Copy JSON'}
        </Button>
      </div>
      <div className="p-3 bg-muted/50 rounded-lg text-sm space-y-2">
        {Object.entries(findings).map(([key, value]) => (
          <div key={key} className="flex gap-2">
            <span className="text-purple-600 dark:text-purple-400 font-medium shrink-0">
              {key}:
            </span>
            <div className="min-w-0 flex-1">
              <SimpleFindingValue value={value} />
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
